package updater

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"go.uber.org/zap"
)

type Updater struct {
	docker      *dockercli.Client
	updateLocks sync.Map // map[string]*sync.Mutex - 每个容器ID对应一个锁
}

func New(d *dockercli.Client) *Updater { return &Updater{docker: d} }

// getContainerLock 获取或创建指定容器的互斥锁
func (u *Updater) getContainerLock(containerID string) *sync.Mutex {
	lockInterface, _ := u.updateLocks.LoadOrStore(containerID, &sync.Mutex{})
	return lockInterface.(*sync.Mutex)
}

// UpdateContainer 拉取镜像并按原配置重建容器，尽量无感更新。
func (u *Updater) UpdateContainer(ctx context.Context, containerID string, imageRef string) error {
	// 获取容器专属锁，防止并发更新同一容器
	mutex := u.getContainerLock(containerID)
	mutex.Lock()
	defer mutex.Unlock()

	logger.Logger.Info("开始更新容器", zap.String("containerID", containerID), zap.String("imageRef", imageRef))

	// 初始化更新上下文
	uctx := &updateContext{
		containerID: containerID,
		imageRef:    imageRef,
	}

	// 1. 拉取镜像
	if err := u.pullImage(ctx, imageRef); err != nil {
		return err
	}

	// 2. 准备旧容器（停止、重命名、清理资源）
	if err := u.prepareOldContainer(ctx, uctx); err != nil {
		return err
	}

	// 3. 创建新容器（带重试）
	if err := u.createContainerWithRetry(ctx, uctx); err != nil {
		u.rollbackOnCreateFailure(ctx, uctx)
		return err
	}

	// 4. 根据旧容器状态决定是否启动新容器
	if uctx.wasRunning {
		// 只有旧容器原来在运行时，才启动新容器
		if err := u.startContainerWithRetry(ctx, uctx); err != nil {
			u.rollbackOnStartFailure(ctx, uctx)
			return err
		}
		logger.Logger.Info("新容器已启动（因为旧容器原来在运行）", zap.String("containerID", uctx.newID))
	} else {
		logger.Logger.Info("新容器已创建但未启动（因为旧容器原来是停止状态）", zap.String("containerID", uctx.newID))
	}

	// 5. 最终清理旧容器及相关资源
	u.finalCleanup(ctx, uctx)
	return nil
}

// updateContext 更新操作的上下文信息
type updateContext struct {
	containerID string
	imageRef    string
	oldInfo     container.InspectResponse
	oldName     string
	backupName  string
	newID       string
	wasRunning  bool // 记录旧容器是否在运行状态
}

const maxRetries = 3

// pullImage 拉取镜像
func (u *Updater) pullImage(ctx context.Context, imageRef string) error {
	logger.Logger.Info("开始拉取镜像", zap.String("imageRef", imageRef))
	err := u.docker.ImagePull(ctx, imageRef)
	if err != nil {
		logger.Logger.Error("拉取镜像失败", zap.String("imageRef", imageRef), zap.Error(err))
		return fmt.Errorf("pull: %w", err)
	}
	logger.Logger.Info("镜像拉取成功", zap.String("imageRef", imageRef))
	return nil
}

// prepareOldContainer 准备旧容器：获取信息、停止、等待、重命名、清理资源
func (u *Updater) prepareOldContainer(ctx context.Context, uctx *updateContext) error {
	// 读取旧容器详细信息与配置
	oldInfo, err := u.docker.InspectContainer(ctx, uctx.containerID)
	if err != nil {
		return fmt.Errorf("inspect: %w", err)
	}
	uctx.oldInfo = oldInfo

	// 记录旧容器的运行状态
	uctx.wasRunning = oldInfo.State.Running
	logger.Logger.Info("旧容器详细信息与配置读取成功",
		zap.String("containerID", uctx.containerID),
		zap.Bool("wasRunning", uctx.wasRunning))

	// 如果旧容器在运行，则停止它
	if uctx.wasRunning {
		err = u.docker.StopContainer(ctx, uctx.containerID, 100)
		if err != nil {
			return fmt.Errorf("停止旧容器失败: %w", err)
		}
		logger.Logger.Info("旧容器停止成功", zap.String("containerID", uctx.containerID))
	} else {
		logger.Logger.Info("旧容器原本就是停止状态，无需停止", zap.String("containerID", uctx.containerID))
	}

	// 等待容器完全停止，确保文件系统完全释放（只对原来运行的容器执行）
	if uctx.wasRunning {
		err = u.docker.WaitContainerStopped(ctx, uctx.containerID, 30)
		if err != nil {
			logger.Logger.Warn("等待容器停止超时，继续执行", zap.String("containerID", uctx.containerID), zap.Error(err))
		}
		logger.Logger.Info("容器完全停止，文件系统已释放", zap.String("containerID", uctx.containerID))
	}

	// 获取容器名称并重命名旧容器
	uctx.oldName = oldInfo.Name
	if len(uctx.oldName) > 0 && uctx.oldName[0] == '/' {
		uctx.oldName = uctx.oldName[1:]
	}
	uctx.backupName = fmt.Sprintf("%s-old-%d", uctx.oldName, time.Now().Unix())
	if uctx.oldName != "" {
		err = u.docker.RenameContainer(ctx, uctx.containerID, uctx.backupName)
		if err != nil {
			return fmt.Errorf("重命名旧容器失败: %w", err)
		}
	}

	// 清理旧容器相关资源，确保没有冲突
	logger.Logger.Info("清理旧容器相关资源，防止创建冲突", zap.String("containerID", uctx.containerID))
	err = u.docker.CleanupContainerResources(ctx, oldInfo)
	if err != nil {
		logger.Logger.Warn("清理旧容器资源失败，继续执行", zap.String("containerID", uctx.containerID), zap.Error(err))
	}

	return nil
}

// createContainerWithRetry 创建新容器（带重试机制）
func (u *Updater) createContainerWithRetry(ctx context.Context, uctx *updateContext) error {
	// 准备新容器配置
	newCfg := uctx.oldInfo.Config
	newCfg.Image = uctx.imageRef
	netCfg := &network.NetworkingConfig{EndpointsConfig: uctx.oldInfo.NetworkSettings.Networks}
	logger.Logger.Info("创建新容器", zap.String("containerName", uctx.oldName), zap.String("imageRef", uctx.imageRef))

	// 尝试创建新容器，增加重试机制
	var createErr error
	for i := 0; i < maxRetries; i++ {
		uctx.newID, createErr = u.docker.CreateContainer(ctx, uctx.oldName, newCfg, uctx.oldInfo.HostConfig, netCfg)
		if createErr == nil {
			break
		}

		logger.Logger.Warn("创建新容器失败，准备重试",
			zap.String("containerName", uctx.oldName),
			zap.Int("attempt", i+1),
			zap.Int("maxRetries", maxRetries),
			zap.Error(createErr))

		if i < maxRetries-1 {
			// 在重试前等待并清理旧容器相关资源
			time.Sleep(time.Duration(i+1) * 2 * time.Second)
			_ = u.docker.CleanupContainerResources(ctx, uctx.oldInfo)
		}
	}

	if createErr != nil {
		return fmt.Errorf("创建新容器多次重试后仍失败: %w", createErr)
	}
	return nil
}

// startContainerWithRetry 启动新容器（带重试机制）
func (u *Updater) startContainerWithRetry(ctx context.Context, uctx *updateContext) error {
	var startErr error
	for i := 0; i < maxRetries; i++ {
		startErr = u.docker.StartContainer(ctx, uctx.newID)
		if startErr == nil {
			break
		}

		logger.Logger.Warn("启动新容器失败，准备重试",
			zap.String("containerID", uctx.newID),
			zap.Int("attempt", i+1),
			zap.Int("maxRetries", maxRetries),
			zap.Error(startErr))

		if i < maxRetries-1 {
			// 在重试前等待并清理旧容器相关资源
			time.Sleep(time.Duration(i+1) * 2 * time.Second)
			_ = u.docker.CleanupContainerResources(ctx, uctx.oldInfo)
		}
	}

	if startErr != nil {
		return fmt.Errorf("启动新容器失败: %w", startErr)
	}
	return nil
}

// rollbackOnCreateFailure 创建失败时的回滚操作
func (u *Updater) rollbackOnCreateFailure(ctx context.Context, uctx *updateContext) {
	if uctx.oldName != "" {
		logger.Logger.Info("回滚容器名称", zap.String("containerID", uctx.containerID), zap.String("oldName", uctx.oldName))
		_ = u.docker.RenameContainer(ctx, uctx.containerID, uctx.oldName)
	}
}

// rollbackOnStartFailure 启动失败时的回滚操作
func (u *Updater) rollbackOnStartFailure(ctx context.Context, uctx *updateContext) {
	logger.Logger.Error("启动新容器多次重试后仍失败，开始回滚",
		zap.String("containerID", uctx.containerID),
		zap.String("newID", uctx.newID),
		zap.String("oldName", uctx.oldName))

	// 删除新容器
	_ = u.docker.RemoveContainerWithVolumes(ctx, uctx.newID, true)

	// 恢复旧容器名称
	if uctx.oldName != "" {
		_ = u.docker.RenameContainer(ctx, uctx.containerID, uctx.oldName)
	}

	// 根据原始状态决定是否重新启动旧容器
	if uctx.wasRunning {
		_ = u.docker.StartContainer(ctx, uctx.containerID)
		logger.Logger.Info("已重新启动旧容器（因为原来在运行）", zap.String("containerID", uctx.containerID))
	}
}

// finalCleanup 最终清理旧容器及相关资源
func (u *Updater) finalCleanup(ctx context.Context, uctx *updateContext) {
	logger.Logger.Info("容器更新成功，开始清理旧容器及相关资源",
		zap.String("oldContainerID", uctx.containerID),
		zap.String("newContainerID", uctx.newID))

	// 先删除旧容器（必须先删除容器，否则镜像检查时会发现容器还在使用镜像而无法删除）
	err := u.docker.RemoveContainerWithVolumes(ctx, uctx.containerID, true)
	if err != nil {
		logger.Logger.Warn("删除旧容器失败，但继续清理资源", zap.String("containerID", uctx.containerID), zap.Error(err))
	} else {
		logger.Logger.Info("旧容器删除成功", zap.String("containerID", uctx.containerID))
	}

	// 删除容器后，再清理旧容器相关的镜像、网络、卷等资源
	logger.Logger.Info("安全清理旧容器的相关资源", zap.String("oldImageID", uctx.oldInfo.Image))
	err = u.docker.CleanupContainerResources(ctx, uctx.oldInfo)
	if err != nil {
		logger.Logger.Warn("清理旧容器资源失败", zap.String("containerID", uctx.containerID), zap.Error(err))
	}

	logger.Logger.Info("容器更新完成", zap.String("containerName", uctx.oldName), zap.String("imageRef", uctx.imageRef))
}
