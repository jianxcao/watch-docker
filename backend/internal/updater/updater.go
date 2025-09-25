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

	// 使用相同名称与原配置创建新容器（仅替换镜像）
	newCfg := oldInfo.Config
	newCfg.Image = imageRef
	netCfg := &network.NetworkingConfig{EndpointsConfig: oldInfo.NetworkSettings.Networks}
	logger.Logger.Info("创建新容器", zap.String("containerID", oldName), zap.String("imageRef", imageRef), zap.Any("newCfg", newCfg), zap.Any("netCfg", netCfg))
	newID, err := u.docker.CreateContainer(ctx, oldName, newCfg, oldInfo.HostConfig, netCfg)
	if err != nil {
		// 创建失败则回滚容器名称
		if oldName != "" {
			logger.Logger.Info("回滚容器名称", zap.String("containerID", containerID), zap.String("oldName", oldName))
			err = u.docker.RenameContainer(ctx, containerID, oldName)
			if err != nil {
				return fmt.Errorf("回滚容器名称失败: %w", err)
			}
		}
		return fmt.Errorf("创建新容器失败: %w", err)
	}

	if err := u.docker.StartContainer(ctx, newID); err != nil {
		// 启动失败回滚：删除新容器，恢复旧容器名称并尝试重启旧容器
		logger.Logger.Info("回滚容器名称", zap.String("containerID", containerID), zap.String("oldName", oldName))
		_ = u.docker.RemoveContainer(ctx, newID, true)
		if oldName != "" {
			_ = u.docker.RenameContainer(ctx, containerID, oldName)
		}
		_ = u.docker.StartContainer(ctx, containerID)
		return fmt.Errorf("启动新容器失败: %w", err)
	}

	// 更新成功：删除旧容器（尽力而为）
	logger.Logger.Info("删除旧容器", zap.String("containerID", containerID))
	_ = u.docker.RemoveContainer(ctx, containerID, true)
	return nil
}
