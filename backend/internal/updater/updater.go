package updater

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/containerd/errdefs"
	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"go.uber.org/zap"
)

type Updater struct {
	docker      *dockercli.Client
	updateLocks sync.Map // map[string]*sync.Mutex - key 为归一化后的 64 位完整容器 ID
}

func New(d *dockercli.Client) *Updater { return &Updater{docker: d} }

func (u *Updater) getContainerLock(containerID string) *sync.Mutex {
	lockInterface, _ := u.updateLocks.LoadOrStore(containerID, &sync.Mutex{})
	return lockInterface.(*sync.Mutex)
}

// UpdateContainer 拉取镜像并按原配置重建容器，返回新容器 ID。
func (u *Updater) UpdateContainer(ctx context.Context, containerID string, imageRef string) (string, error) {
	return u.UpdateContainerWithProgress(ctx, containerID, imageRef, nil)
}

// UpdateProgressCallback 更新进度回调
type UpdateProgressCallback struct {
	OnStep         func(step, message string)
	OnPullProgress func(progress dockercli.PullProgress)
}

// UpdateContainerWithProgress 带进度回调的容器更新，返回新容器 ID。
func (u *Updater) UpdateContainerWithProgress(ctx context.Context, containerID, imageRef string, cb *UpdateProgressCallback) (string, error) {
	// 先 inspect 获取规范化后的 64 位完整容器 ID 再加锁，确保定时任务与手动触发锁到同一容器
	canonicalID := containerID
	if info, err := u.docker.InspectContainer(ctx, containerID); err == nil {
		canonicalID = info.ID
	}

	mutex := u.getContainerLock(canonicalID)
	mutex.Lock()
	defer mutex.Unlock()

	logger.Logger.Info("开始更新容器", zap.String("containerID", canonicalID), zap.String("imageRef", imageRef))

	// 加锁后重新 inspect，防止等待锁期间容器已被并发更新/删除后继续误拉取
	lockedInfo, err := u.docker.InspectContainer(ctx, canonicalID)
	if err != nil {
		return "", fmt.Errorf("inspect: %w", err)
	}

	var targetPlatform *ocispec.Platform
	if lockedInfo.ImageManifestDescriptor != nil && lockedInfo.ImageManifestDescriptor.Platform != nil &&
		lockedInfo.ImageManifestDescriptor.Platform.OS != "" && lockedInfo.ImageManifestDescriptor.Platform.Architecture != "" {
		p := *lockedInfo.ImageManifestDescriptor.Platform
		targetPlatform = &p
	} else if lockedInfo.Image != "" {
		if imgInfo, ierr := u.docker.ImageInspect(ctx, lockedInfo.Image); ierr == nil && imgInfo.Os != "" && imgInfo.Architecture != "" {
			targetPlatform = &ocispec.Platform{
				OS:           imgInfo.Os,
				Architecture: imgInfo.Architecture,
				Variant:      imgInfo.Variant,
			}
		}
	}

	uctx := &updateContext{
		containerID:    lockedInfo.ID,
		imageRef:       imageRef,
		targetPlatform: targetPlatform,
	}
	if uctx.imageRef == "" && lockedInfo.Config != nil {
		uctx.imageRef = lockedInfo.Config.Image
	}

	// 1. 拉取镜像（保留原容器平台架构）
	if cb != nil && cb.OnStep != nil {
		cb.OnStep("pulling", "正在拉取镜像 "+uctx.imageRef)
	}
	if cb != nil && cb.OnPullProgress != nil {
		err := u.docker.ImagePullWithPlatform(ctx, uctx.imageRef, uctx.targetPlatform, cb.OnPullProgress)
		if err != nil {
			logger.Logger.Error("拉取镜像失败", zap.String("imageRef", uctx.imageRef), zap.Error(err))
			return "", fmt.Errorf("pull: %w", err)
		}
	} else {
		if err := u.pullImageWithPlatform(ctx, uctx.imageRef, uctx.targetPlatform); err != nil {
			return "", err
		}
	}

	// 2. 准备旧容器（先读取完整配置与旧镜像默认配置、再停止与重命名）
	if cb != nil && cb.OnStep != nil {
		cb.OnStep("stopping", "正在停止旧容器")
	}
	stoppedAutoRemoved, err := u.prepareOldContainer(ctx, uctx)
	if err != nil {
		return "", err
	}

	// 3. 创建新容器（带重试），并在启动前附加额外网络
	if cb != nil && cb.OnStep != nil {
		cb.OnStep("creating", "正在创建新容器")
	}
	if err := u.createContainerWithRetry(ctx, uctx); err != nil {
		if !stoppedAutoRemoved {
			u.rollbackOnCreateFailure(ctx, uctx)
		}
		return "", err
	}

	// 4. 根据旧容器状态决定是否启动新容器
	if uctx.wasRunning {
		if cb != nil && cb.OnStep != nil {
			cb.OnStep("starting", "正在启动新容器")
		}
		if err := u.startContainerWithRetry(ctx, uctx); err != nil {
			if !stoppedAutoRemoved {
				u.rollbackOnStartFailure(ctx, uctx)
			} else {
				rbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
				_ = u.docker.RemoveContainer(rbCtx, uctx.newID, true)
				cancel()
			}
			return "", err
		}
		logger.Logger.Info("新容器已启动（因为旧容器原来在运行）", zap.String("containerID", uctx.newID))
	} else {
		logger.Logger.Info("新容器已创建但未启动（因为旧容器原来是停止状态）", zap.String("containerID", uctx.newID))
	}

	// 5. 最终清理旧容器与旧镜像（脱离调用方 ctx，避免新容器已启动后因请求超时遗留 *-old-* 备份容器）
	if cb != nil && cb.OnStep != nil {
		cb.OnStep("cleanup", "正在清理旧容器")
	}
	cleanCtx, cleanCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cleanCancel()
	if !stoppedAutoRemoved {
		u.finalCleanup(cleanCtx, uctx)
	} else {
		// --rm 容器已由 daemon 自动删除，仅尝试清理不再被使用的旧镜像
		if uctx.oldInfo.Image != "" {
			_ = u.docker.SafeRemoveImage(cleanCtx, uctx.oldInfo.Image)
		}
	}

	return uctx.newID, nil
}

type updateContext struct {
	containerID   string
	imageRef      string
	oldInfo       container.InspectResponse
	oldImage      *image.InspectResponse
	oldName       string
	backupName     string
	newID          string
	wasRunning     bool
	extraNetworks  map[string]*network.EndpointSettings
	targetPlatform *ocispec.Platform
}

const maxRetries = 3

func (u *Updater) pullImageWithPlatform(ctx context.Context, imageRef string, platform *ocispec.Platform) error {
	logger.Logger.Info("开始拉取镜像", zap.String("imageRef", imageRef))
	if err := u.docker.ImagePullWithPlatform(ctx, imageRef, platform, nil); err != nil {
		logger.Logger.Error("拉取镜像失败", zap.String("imageRef", imageRef), zap.Error(err))
		return fmt.Errorf("pull: %w", err)
	}
	logger.Logger.Info("镜像拉取成功", zap.String("imageRef", imageRef))
	return nil
}

func (u *Updater) restartOldIfStopped(ctx context.Context, uctx *updateContext) {
	if !uctx.wasRunning {
		return
	}
	rbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	defer cancel()
	info, err := u.docker.InspectContainer(rbCtx, uctx.containerID)
	if err != nil {
		return
	}
	if info.State == nil || !info.State.Running {
		if sErr := u.docker.StartContainer(rbCtx, uctx.containerID); sErr != nil {
			logger.Logger.Error("prepare 失败后恢复启动旧容器失败", zap.String("containerID", uctx.containerID), zap.Error(sErr))
		} else {
			logger.Logger.Info("prepare 失败后已恢复启动旧容器", zap.String("containerID", uctx.containerID))
		}
	}
}

// prepareOldContainer 读取信息、在停止前保存旧名称（兼容 --rm 容器）、停止并重命名旧容器。
func (u *Updater) prepareOldContainer(ctx context.Context, uctx *updateContext) (autoRemoved bool, err error) {
	oldInfo, err := u.docker.InspectContainer(ctx, uctx.containerID)
	if err != nil {
		return false, fmt.Errorf("inspect: %w", err)
	}
	uctx.oldInfo = oldInfo
	if oldInfo.State != nil {
		uctx.wasRunning = oldInfo.State.Running
	}

	// 在 StopContainer 之前记录容器名，防止 AutoRemove(--rm) 容器停止即销毁导致名字丢失
	uctx.oldName = strings.TrimPrefix(oldInfo.Name, "/")

	// 读取旧镜像配置以便后续剥离旧镜像自带默认值（让新镜像自己的 Entrypoint/Cmd/Env/Healthcheck 生效）
	if oldInfo.Image != "" {
		if imgInfo, ierr := u.docker.ImageInspect(ctx, oldInfo.Image); ierr == nil {
			uctx.oldImage = imgInfo
		}
	}

	if uctx.wasRunning {
		if err = u.docker.StopContainer(ctx, uctx.containerID, 100); err != nil {
			u.restartOldIfStopped(ctx, uctx)
			return false, fmt.Errorf("stop: %w", err)
		}
		if err = u.docker.WaitContainerStopped(ctx, uctx.containerID, 30); err != nil {
			if ctx.Err() != nil {
				u.restartOldIfStopped(ctx, uctx)
				return false, fmt.Errorf("stop wait: %w", err)
			}
			logger.Logger.Warn("等待容器停止异常，继续检查", zap.String("containerID", uctx.containerID), zap.Error(err))
		}
	}

	if err := ctx.Err(); err != nil {
		u.restartOldIfStopped(ctx, uctx)
		return false, err
	}

	// 使用脱离调用方 ctx 的检查上下文，仅确认 errdefs.IsNotFound 时才判定为 --rm 自动移除
	checkCtx, checkCancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	_, inspErr := u.docker.InspectContainer(checkCtx, uctx.containerID)
	checkCancel()
	if inspErr != nil {
		if errdefs.IsNotFound(inspErr) {
			logger.Logger.Info("旧容器停止后已不存在（可能为 --rm 自动移除），直接按保存配置重建",
				zap.String("containerID", uctx.containerID),
				zap.String("oldName", uctx.oldName))
			return true, nil
		}
		u.restartOldIfStopped(ctx, uctx)
		return false, fmt.Errorf("post-stop inspect: %w", inspErr)
	}

	uctx.backupName = fmt.Sprintf("%s-old-%d", uctx.oldName, time.Now().UnixNano())
	if uctx.oldName != "" {
		if err = u.docker.RenameContainer(ctx, uctx.containerID, uctx.backupName); err != nil {
			u.restartOldIfStopped(ctx, uctx)
			return false, fmt.Errorf("重命名旧容器失败: %w", err)
		}
	}

	return false, nil
}

func splitBindSourceAndTarget(bind string) (source, target string) {
	parts := strings.Split(bind, ":")
	if len(parts) < 2 {
		return "", ""
	}
	// 兼容 Windows 宿主机盘符路径 (e.g. C:\data:/app[:ro])
	if len(parts[0]) == 1 && len(parts) >= 3 && (strings.HasPrefix(parts[1], "\\") || strings.HasPrefix(parts[1], "/")) {
		return parts[0] + ":" + parts[1], parts[2]
	}
	return parts[0], parts[1]
}

func cloneExposedPorts(src network.PortSet) network.PortSet {
	if len(src) == 0 {
		return nil
	}
	out := make(network.PortSet, len(src))
	for p := range src {
		out[p] = struct{}{}
	}
	return out
}

func (u *Updater) buildNewContainerConfigs(uctx *updateContext) (*container.Config, *container.HostConfig, *network.NetworkingConfig, *ocispec.Platform, map[string]*network.EndpointSettings) {
	// 1. 克隆并清洗 Container Config，剥离与旧镜像默认值完全相同的字段
	var newCfg container.Config
	if uctx.oldInfo.Config != nil {
		newCfg = *uctx.oldInfo.Config
		if uctx.oldInfo.Config.Labels != nil {
			newCfg.Labels = maps.Clone(uctx.oldInfo.Config.Labels)
		}
		newCfg.ExposedPorts = cloneExposedPorts(uctx.oldInfo.Config.ExposedPorts)
	}
	newCfg.Image = uctx.imageRef

	var imgExposed map[string]struct{}
	if uctx.oldImage != nil && uctx.oldImage.Config != nil {
		imgCfg := uctx.oldImage.Config
		imgExposed = imgCfg.ExposedPorts

		// 保留用户显式 --entrypoint "" 对镜像默认 Entrypoint 的清空覆盖
		entryOverridden := false
		if len(imgCfg.Entrypoint) > 0 && len(newCfg.Entrypoint) == 0 {
			newCfg.Entrypoint = []string{""}
			entryOverridden = true
		} else if reflect.DeepEqual(newCfg.Entrypoint, imgCfg.Entrypoint) {
			newCfg.Entrypoint = nil
		} else if len(newCfg.Entrypoint) > 0 {
			entryOverridden = true
		}

		if !entryOverridden && reflect.DeepEqual(newCfg.Cmd, imgCfg.Cmd) {
			newCfg.Cmd = nil
		}
		if newCfg.WorkingDir == imgCfg.WorkingDir {
			newCfg.WorkingDir = ""
		}
		if newCfg.User == imgCfg.User {
			newCfg.User = ""
		}
		if imgCfg.Healthcheck != nil && reflect.DeepEqual(newCfg.Healthcheck, imgCfg.Healthcheck) {
			newCfg.Healthcheck = nil
		}
		if len(imgCfg.Env) > 0 && len(newCfg.Env) > 0 {
			imgEnvSet := make(map[string]struct{}, len(imgCfg.Env))
			for _, e := range imgCfg.Env {
				imgEnvSet[e] = struct{}{}
			}
			filteredEnv := make([]string, 0, len(newCfg.Env))
			for _, e := range newCfg.Env {
				if _, isImgDefault := imgEnvSet[e]; !isImgDefault {
					filteredEnv = append(filteredEnv, e)
				}
			}
			newCfg.Env = filteredEnv
		}
		// 剥离与旧镜像完全相同的 LABEL，避免覆盖新镜像版本元信息
		if len(imgCfg.Labels) > 0 && len(newCfg.Labels) > 0 {
			for k, v := range newCfg.Labels {
				if strings.HasPrefix(k, "com.docker.compose.") || strings.HasPrefix(k, "watchdocker.") {
					continue
				}
				if imgVal, ok := imgCfg.Labels[k]; ok && imgVal == v {
					delete(newCfg.Labels, k)
				}
			}
		}
	}

	// 2. 克隆 HostConfig 并显式迁移匿名卷挂载，确保数据不丢失
	var newHostCfg container.HostConfig
	if uctx.oldInfo.HostConfig != nil {
		newHostCfg = *uctx.oldInfo.HostConfig
		newHostCfg.Mounts = append([]mount.Mount(nil), uctx.oldInfo.HostConfig.Mounts...)
		newHostCfg.Binds = append([]string(nil), uctx.oldInfo.HostConfig.Binds...)
		if uctx.oldInfo.HostConfig.PortBindings != nil {
			newHostCfg.PortBindings = maps.Clone(uctx.oldInfo.HostConfig.PortBindings)
		}
	}

	if !newHostCfg.AutoRemove && len(newHostCfg.VolumesFrom) == 0 && len(newHostCfg.Binds) > 0 {
		volByDest := make(map[string]struct {
			name string
			rw   bool
			dest string
		}, len(uctx.oldInfo.Mounts))
		for _, m := range uctx.oldInfo.Mounts {
			volName := dockercli.ResolveMountVolumeName(m)
			if volName != "" && m.Destination != "" {
				volByDest[m.Destination] = struct {
					name string
					rw   bool
					dest string
				}{name: volName, rw: m.RW, dest: m.Destination}
			}
		}
		for i, b := range newHostCfg.Binds {
			if strings.Contains(b, ":") {
				continue
			}
			v, ok := volByDest[b]
			if !ok && strings.HasPrefix(b, "/") && b != "/" {
				v, ok = volByDest[strings.TrimRight(b, "/")]
			}
			if !ok {
				continue
			}
			rewritten := v.name + ":" + v.dest
			if !v.rw {
				rewritten += ":ro"
			}
			newHostCfg.Binds[i] = rewritten
		}
	}

	existingTargets := make(map[string]struct{})
	existingSources := make(map[string]struct{})
	for _, m := range newHostCfg.Mounts {
		if m.Target != "" {
			existingTargets[m.Target] = struct{}{}
		}
		if m.Source != "" {
			existingSources[m.Source] = struct{}{}
		}
	}
	for _, b := range newHostCfg.Binds {
		src, tgt := splitBindSourceAndTarget(b)
		if tgt != "" {
			existingTargets[tgt] = struct{}{}
		}
		if src != "" {
			existingSources[src] = struct{}{}
		}
	}

	if !newHostCfg.AutoRemove && len(newHostCfg.VolumesFrom) == 0 {
		for _, m := range uctx.oldInfo.Mounts {
			volName := dockercli.ResolveMountVolumeName(m)
			if volName != "" && m.Destination != "" {
				_, hasTarget := existingTargets[m.Destination]
				_, hasSource := existingSources[volName]
				if !hasTarget && !hasSource {
					newHostCfg.Mounts = append(newHostCfg.Mounts, mount.Mount{
						Type:     mount.TypeVolume,
						Source:   volName,
						Target:   m.Destination,
						ReadOnly: !m.RW,
					})
					existingTargets[m.Destination] = struct{}{}
					existingSources[volName] = struct{}{}
				}
			}
		}
	}

	// 3. 构造干净的网络配置：处理 container:<id>/none/host 端口冲突、保留原主网络与 bridge 约束
	var netCfg *network.NetworkingConfig
	var extraNetworks map[string]*network.EndpointSettings

	netMode := newHostCfg.NetworkMode
	if netMode.IsContainer() || netMode.IsNone() {
		newCfg.ExposedPorts = nil
		newHostCfg.PortBindings = nil
		newHostCfg.PublishAllPorts = false
		if netMode.IsContainer() {
			newCfg.Hostname = ""
			newCfg.Domainname = ""
		}
	} else {
		// 仅保留用户在 PortBindings 引用的端口或镜像本身未声明的额外 EXPOSE 端口
		if len(imgExposed) > 0 && len(newCfg.ExposedPorts) > 0 {
			for rawPort := range imgExposed {
				portSpec := rawPort
				if !strings.Contains(portSpec, "/") {
					portSpec += "/tcp"
				}
				if p, err := network.ParsePort(portSpec); err == nil {
					if _, bound := newHostCfg.PortBindings[p]; !bound {
						delete(newCfg.ExposedPorts, p)
					}
				}
			}
			if len(newCfg.ExposedPorts) == 0 {
				newCfg.ExposedPorts = nil
			}
		}

		if !netMode.IsHost() && uctx.oldInfo.NetworkSettings != nil && len(uctx.oldInfo.NetworkSettings.Networks) > 0 {
			shortOld := uctx.containerID
			if len(shortOld) > 12 {
				shortOld = shortOld[:12]
			}

			netNames := make([]string, 0, len(uctx.oldInfo.NetworkSettings.Networks))
			for name := range uctx.oldInfo.NetworkSettings.Networks {
				netNames = append(netNames, name)
			}
			sort.Strings(netNames)

			origPrimary := strings.TrimSpace(string(netMode))
			if origPrimary == "" || origPrimary == "default" {
				if _, hasBridge := uctx.oldInfo.NetworkSettings.Networks["bridge"]; hasBridge {
					origPrimary = "bridge"
				}
			}
			primaryName := netNames[0]
			if _, ok := uctx.oldInfo.NetworkSettings.Networks[origPrimary]; ok {
				primaryName = origPrimary
			} else if origPrimary != "" {
				for _, name := range netNames {
					ep := uctx.oldInfo.NetworkSettings.Networks[name]
					if ep != nil && ep.NetworkID != "" && (ep.NetworkID == origPrimary || (len(origPrimary) >= 12 && strings.HasPrefix(ep.NetworkID, origPrimary))) {
						primaryName = name
						break
					}
				}
			}
			newHostCfg.NetworkMode = container.NetworkMode(primaryName)

			sanitizeEndpoint := func(netName string, ep *network.EndpointSettings) *network.EndpointSettings {
				if ep == nil {
					return &network.EndpointSettings{}
				}
				var mac network.HardwareAddr
				if len(ep.MacAddress) > 0 {
					mac = slices.Clone(ep.MacAddress)
				}
				if netName == "bridge" {
					// Docker 26+ 禁止默认 bridge 网络携带 Aliases/IPAMConfig/Links
					return &network.EndpointSettings{
						DriverOpts: maps.Clone(ep.DriverOpts),
						MacAddress: mac,
					}
				}
				cleanEp := &network.EndpointSettings{
					Links:      append([]string(nil), ep.Links...),
					DriverOpts: maps.Clone(ep.DriverOpts),
					GwPriority: ep.GwPriority,
					MacAddress: mac,
				}
				if ep.IPAMConfig != nil {
					cleanEp.IPAMConfig = ep.IPAMConfig.Copy()
				}
				for _, alias := range ep.Aliases {
					if alias != "" && alias != shortOld && alias != uctx.containerID {
						cleanEp.Aliases = append(cleanEp.Aliases, alias)
					}
				}
				return cleanEp
			}

			netCfg = &network.NetworkingConfig{
				EndpointsConfig: map[string]*network.EndpointSettings{
					primaryName: sanitizeEndpoint(primaryName, uctx.oldInfo.NetworkSettings.Networks[primaryName]),
				},
			}
			if len(netNames) > 1 {
				extraNetworks = make(map[string]*network.EndpointSettings, len(netNames)-1)
				for _, name := range netNames {
					if name == primaryName {
						continue
					}
					extraNetworks[name] = sanitizeEndpoint(name, uctx.oldInfo.NetworkSettings.Networks[name])
				}
			}
		}
	}

	var platform *ocispec.Platform
	if uctx.targetPlatform != nil && uctx.targetPlatform.OS != "" && uctx.targetPlatform.Architecture != "" {
		p := *uctx.targetPlatform
		platform = &p
	} else if uctx.oldImage != nil && uctx.oldImage.Os != "" && uctx.oldImage.Architecture != "" {
		platform = &ocispec.Platform{
			OS:           uctx.oldImage.Os,
			Architecture: uctx.oldImage.Architecture,
			Variant:      uctx.oldImage.Variant,
		}
	}

	return &newCfg, &newHostCfg, netCfg, platform, extraNetworks
}

func (u *Updater) createContainerWithRetry(ctx context.Context, uctx *updateContext) error {
	newCfg, newHostCfg, netCfg, platform, extraNetworks := u.buildNewContainerConfigs(uctx)
	uctx.extraNetworks = extraNetworks

	logger.Logger.Info("创建新容器", zap.String("containerName", uctx.oldName), zap.String("imageRef", uctx.imageRef))

	var createErr error
	for i := 0; i < maxRetries; i++ {
		uctx.newID, createErr = u.docker.CreateContainerWithPlatform(ctx, uctx.oldName, newCfg, newHostCfg, netCfg, platform)
		if createErr == nil {
			break
		}

		logger.Logger.Warn("创建新容器失败，准备重试",
			zap.String("containerName", uctx.oldName),
			zap.Int("attempt", i+1),
			zap.Int("maxRetries", maxRetries),
			zap.Error(createErr))

		if i < maxRetries-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(i+1) * 2 * time.Second):
			}
		}
	}

	if createErr != nil {
		return fmt.Errorf("create: 创建新容器多次重试后仍失败: %w", createErr)
	}

	if len(uctx.extraNetworks) > 0 {
		extraNames := make([]string, 0, len(uctx.extraNetworks))
		for netName := range uctx.extraNetworks {
			extraNames = append(extraNames, netName)
		}
		sort.Strings(extraNames)
		for _, netName := range extraNames {
			if err := u.docker.ConnectContainerToNetwork(ctx, netName, uctx.newID, uctx.extraNetworks[netName]); err != nil {
				logger.Logger.Error("附加额外网络失败",
					zap.String("network", netName),
					zap.String("newContainerID", uctx.newID),
					zap.Error(err))
				// 清理未完整创建的新容器，交由外层回滚
				rbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
				_ = u.docker.RemoveContainer(rbCtx, uctx.newID, true)
				cancel()
				return fmt.Errorf("create: 连接附加网络 %s 失败: %w", netName, err)
			}
		}
	}

	return nil
}

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
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(i+1) * 2 * time.Second):
			}
		}
	}

	if startErr != nil {
		return fmt.Errorf("start new: 启动新容器失败: %w", startErr)
	}
	return nil
}

// rollbackOnCreateFailure 创建失败时的回滚操作（脱离外部 ctx 取消，并在原运行中时恢复启动）
func (u *Updater) rollbackOnCreateFailure(ctx context.Context, uctx *updateContext) {
	rbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cancel()

	if uctx.oldName != "" {
		logger.Logger.Info("回滚旧容器名称", zap.String("containerID", uctx.containerID), zap.String("oldName", uctx.oldName))
		if err := u.docker.RenameContainer(rbCtx, uctx.containerID, uctx.oldName); err != nil {
			logger.Logger.Error("回滚容器名称失败", zap.String("containerID", uctx.containerID), zap.Error(err))
		}
	}
	if uctx.wasRunning {
		if err := u.docker.StartContainer(rbCtx, uctx.containerID); err != nil {
			logger.Logger.Error("回滚重启旧容器失败", zap.String("containerID", uctx.containerID), zap.Error(err))
		} else {
			logger.Logger.Info("创建失败回滚：已重启旧容器", zap.String("containerID", uctx.containerID))
		}
	}
}

// rollbackOnStartFailure 启动失败时的回滚操作（脱离外部 ctx 取消）
func (u *Updater) rollbackOnStartFailure(ctx context.Context, uctx *updateContext) {
	rbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cancel()

	logger.Logger.Error("启动新容器失败，开始回滚",
		zap.String("containerID", uctx.containerID),
		zap.String("newID", uctx.newID),
		zap.String("oldName", uctx.oldName))

	// 不带 RemoveVolumes 删除新容器，避免误删已迁移给新容器的匿名卷
	_ = u.docker.RemoveContainer(rbCtx, uctx.newID, true)

	if uctx.oldName != "" {
		_ = u.docker.RenameContainer(rbCtx, uctx.containerID, uctx.oldName)
	}

	if uctx.wasRunning {
		if err := u.docker.StartContainer(rbCtx, uctx.containerID); err != nil {
			logger.Logger.Error("回滚重启旧容器失败", zap.String("containerID", uctx.containerID), zap.Error(err))
		} else {
			logger.Logger.Info("已重新启动旧容器（因为原来在运行）", zap.String("containerID", uctx.containerID))
		}
	}
}

// finalCleanup 最终清理旧容器及相关资源
func (u *Updater) finalCleanup(ctx context.Context, uctx *updateContext) {
	logger.Logger.Info("容器更新成功，开始清理旧容器",
		zap.String("oldContainerID", uctx.containerID),
		zap.String("newContainerID", uctx.newID))

	// 必须使用 RemoveVolumes=false（RemoveContainer），因为旧容器的匿名卷已由新容器挂载继承！
	if err := u.docker.RemoveContainer(ctx, uctx.containerID, true); err != nil {
		logger.Logger.Warn("删除旧容器失败", zap.String("containerID", uctx.containerID), zap.Error(err))
	} else {
		logger.Logger.Info("旧容器删除成功", zap.String("containerID", uctx.containerID))
	}

	// 清理不再被使用的旧镜像与真正孤立的匿名卷
	if err := u.docker.CleanupContainerResources(ctx, uctx.oldInfo); err != nil {
		logger.Logger.Warn("清理旧容器资源失败", zap.String("containerID", uctx.containerID), zap.Error(err))
	}

	logger.Logger.Info("容器更新完成", zap.String("containerName", uctx.oldName), zap.String("imageRef", uctx.imageRef), zap.String("newID", uctx.newID))
}
