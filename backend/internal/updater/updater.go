package updater

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"

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
// 步骤：
// 1) 拉取目标镜像 imageRef（尽力而为）
// 2) 检查旧容器，记录其 Config/HostConfig/Networking 配置
// 3) 优雅停止并重命名旧容器（释放原名称）
// 4) 使用相同名称和原配置创建新容器（镜像替换为 imageRef）
// 5) 启动新容器；若失败则回滚：删除新容器、恢复旧容器名称并重新启动
func (u *Updater) UpdateContainer(ctx context.Context, containerID string, imageRef string) error {
	// 获取容器专属锁，防止并发更新同一容器
	mutex := u.getContainerLock(containerID)
	mutex.Lock()
	defer mutex.Unlock()

	logger.Logger.Info("开始更新容器", zap.String("containerID", containerID), zap.String("imageRef", imageRef))
	logger.Logger.Info("开始拉取镜像", zap.String("imageRef", imageRef))
	// 尝试先拉取镜像（忽略错误，后续启动失败会回滚）
	err := u.docker.ImagePull(ctx, imageRef)
	if err != nil {
		logger.Logger.Error("拉取镜像失败", zap.String("imageRef", imageRef), zap.Error(err))
		return fmt.Errorf("pull: %w", err)
	}
	logger.Logger.Info("镜像拉取成功", zap.String("imageRef", imageRef))
	// 读取旧容器详细信息与配置
	oldInfo, err := u.docker.InspectContainer(ctx, containerID)
	if err != nil {
		return fmt.Errorf("inspect: %w", err)
	}
	logger.Logger.Info("旧容器详细信息与配置读取成功", zap.String("containerID", containerID))
	// 尽量优雅停止旧容器
	err = u.docker.StopContainer(ctx, containerID, 100)
	if err != nil {
		return fmt.Errorf("停止旧容器失败: %w", err)
	}
	logger.Logger.Info("旧容器停止成功", zap.String("containerID", containerID))

	// 等待容器完全停止，确保文件系统完全释放
	err = u.docker.WaitContainerStopped(ctx, containerID, 30)
	if err != nil {
		logger.Logger.Warn("等待容器停止超时，继续执行", zap.String("containerID", containerID), zap.Error(err))
	}
	logger.Logger.Info("容器完全停止，文件系统已释放", zap.String("containerID", containerID))
	// 重命名旧容器，释放原有容器名称
	oldName := oldInfo.Name
	if len(oldName) > 0 && oldName[0] == '/' {
		oldName = oldName[1:]
	}
	backupName := fmt.Sprintf("%s-old-%d", oldName, time.Now().Unix())
	if oldName != "" {
		err = u.docker.RenameContainer(ctx, containerID, backupName)
		if err != nil {
			return fmt.Errorf("重命名旧容器失败: %w", err)
		}
	}

	// 清理悬挂的文件系统，确保Docker内部状态干净
	logger.Logger.Info("清理悬挂的文件系统", zap.String("containerID", containerID))
	err = u.docker.PruneSystem(ctx)
	if err != nil {
		logger.Logger.Warn("清理悬挂文件系统失败，继续执行", zap.String("containerID", containerID), zap.Error(err))
	}

	// 使用相同名称与原配置创建新容器（仅替换镜像）
	newCfg := oldInfo.Config
	newCfg.Image = imageRef
	netCfg := &network.NetworkingConfig{EndpointsConfig: oldInfo.NetworkSettings.Networks}
	logger.Logger.Info("创建新容器", zap.String("containerID", oldName), zap.String("imageRef", imageRef), zap.Any("newCfg", newCfg), zap.Any("netCfg", netCfg))

	// 尝试创建新容器，增加重试机制
	var newID string
	var createErr error
	const maxRetries = 3
	for i := 0; i < maxRetries; i++ {
		newID, createErr = u.docker.CreateContainer(ctx, oldName, newCfg, oldInfo.HostConfig, netCfg)
		if createErr == nil {
			break
		}
		logger.Logger.Warn("创建新容器失败，准备重试",
			zap.String("containerName", oldName),
			zap.Int("attempt", i+1),
			zap.Int("maxRetries", maxRetries),
			zap.Error(createErr))

		if i < maxRetries-1 {
			// 在重试前等待并清理
			time.Sleep(time.Duration(i+1) * 2 * time.Second)
			_ = u.docker.PruneSystem(ctx)
		}
	}

	if createErr != nil {
		// 创建失败则回滚容器名称
		if oldName != "" {
			logger.Logger.Info("回滚容器名称", zap.String("containerID", containerID), zap.String("oldName", oldName))
			rollbackErr := u.docker.RenameContainer(ctx, containerID, oldName)
			if rollbackErr != nil {
				return fmt.Errorf("回滚容器名称失败: %w", rollbackErr)
			}
		}
		return fmt.Errorf("创建新容器多次重试后仍失败: %w", createErr)
	}

	// 尝试启动新容器，增加重试机制
	var startErr error
	for i := 0; i < maxRetries; i++ {
		startErr = u.docker.StartContainer(ctx, newID)
		if startErr == nil {
			break
		}

		logger.Logger.Warn("启动新容器失败，准备重试",
			zap.String("containerID", newID),
			zap.Int("attempt", i+1),
			zap.Int("maxRetries", maxRetries),
			zap.Error(startErr))

		if i < maxRetries-1 {
			// 在重试前等待并清理
			time.Sleep(time.Duration(i+1) * 2 * time.Second)
			_ = u.docker.PruneSystem(ctx)
		}
	}

	if startErr != nil {
		// 启动失败回滚：删除新容器，恢复旧容器名称并尝试重启旧容器
		logger.Logger.Error("启动新容器多次重试后仍失败，开始回滚",
			zap.String("containerID", containerID),
			zap.String("newID", newID),
			zap.String("oldName", oldName),
			zap.Error(startErr))
		_ = u.docker.RemoveContainerWithVolumes(ctx, newID, true)
		if oldName != "" {
			_ = u.docker.RenameContainer(ctx, containerID, oldName)
		}
		_ = u.docker.StartContainer(ctx, containerID)
		return fmt.Errorf("启动新容器失败: %w", startErr)
	}

	// 更新成功：删除旧容器并清理关联卷（尽力而为）
	logger.Logger.Info("容器更新成功，删除旧容器", zap.String("oldContainerID", containerID), zap.String("newContainerID", newID))
	_ = u.docker.RemoveContainerWithVolumes(ctx, containerID, true)
	logger.Logger.Info("容器更新完成", zap.String("containerName", oldName), zap.String("imageRef", imageRef))
	return nil
}
