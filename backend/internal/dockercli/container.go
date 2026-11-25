package dockercli

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"go.uber.org/zap"
)

type ContainerInfo struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Image        string            `json:"image"`
	ImageID      string            `json:"imageId"`
	RepoTags     []string          `json:"repoTags"`
	RepoDigests  []string          `json:"repoDigests"`
	IsLocalImage bool              `json:"isLocalImage"`
	Labels       map[string]string `json:"labels"`
	State        string            `json:"state"`
	Status       string            `json:"status"`
	Created      int64             `json:"created"`
	StartedAt    string            `json:"startedAt"` // 容器启动时间
	Ports        []PortInfo        `json:"ports"`     // 端口映射信息

	// 以下字段仅供后端扫描匹配多架构镜像与 config digest，不序列化到前端
	ImageOS      string `json:"-"`
	ImageArch    string `json:"-"`
	ImageVariant string `json:"-"`
}

// PortInfo 端口映射信息
type PortInfo struct {
	IP          string `json:"ip"`          // 主机IP地址
	PrivatePort int    `json:"privatePort"` // 容器内部端口
	PublicPort  int    `json:"publicPort"`  // 主机端口
	Type        string `json:"type"`        // 端口类型 (tcp/udp)
}

// ListContainers 列出容器并附带镜像的 RepoTags/RepoDigests（通过带缓存的 ImageInspect 合并）
func (c *Client) ListContainers(ctx context.Context, includeStopped bool) ([]ContainerInfo, error) {
	listRes, err := c.docker.ContainerList(ctx, client.ContainerListOptions{All: includeStopped})
	if err != nil {
		return nil, err
	}
	containers := listRes.Items

	// 先收集需要 inspect 的镜像 ID，避免重复请求
	imageIDSet := make(map[string]struct{})
	for _, ct := range containers {
		if ct.ImageID != "" {
			imageIDSet[ct.ImageID] = struct{}{}
		}
	}

	// 先按需逐个 inspect 并用已有元数据分类，仅在确实需要 ParentID 时才回查镜像列表
	imageTags := make(map[string][]string, len(imageIDSet))
	imageDigests := make(map[string][]string, len(imageIDSet))
	imageLocalFlags := make(map[string]bool, len(imageIDSet))
	imagePlatforms := make(map[string][3]string, len(imageIDSet))
	pendingParent := make(map[string]time.Time)
	for imageID := range imageIDSet {
		meta, ierr := c.inspectImageMeta(ctx, imageID)
		if ierr != nil {
			continue
		}
		img := meta.Inspect
		imageTags[imageID] = img.RepoTags
		if digests := comparableRepoDigests(img); len(digests) > 0 {
			imageDigests[imageID] = digests
		}
		imagePlatforms[imageID] = [3]string{img.Os, img.Architecture, img.Variant}

		local, needsParent := classifyLocalImage(img, imageDigests[imageID])
		if !needsParent {
			imageLocalFlags[imageID] = local
			continue
		}
		if meta.ParentKnown {
			imageLocalFlags[imageID] = strings.TrimSpace(meta.ParentID) != ""
			continue
		}
		pendingParent[imageID] = meta.FetchedAt
	}

	if len(pendingParent) > 0 {
		parentMap := make(map[string]string, len(pendingParent))
		if imgListRes, lerr := c.docker.ImageList(ctx, client.ImageListOptions{All: true}); lerr == nil {
			for _, im := range imgListRes.Items {
				parentMap[im.ID] = im.ParentID
			}
			for imageID, fetchedAt := range pendingParent {
				if parentID, ok := parentMap[imageID]; ok {
					c.setCachedImageParent(imageID, fetchedAt, parentID)
					imageLocalFlags[imageID] = strings.TrimSpace(parentID) != ""
				} else {
					imageLocalFlags[imageID] = false
				}
			}
		} else {
			for imageID := range pendingParent {
				imageLocalFlags[imageID] = false
			}
		}
	}

	// 清理已消失容器的 startedAt 缓存
	activeIDs := make(map[string]struct{}, len(containers))
	for _, ct := range containers {
		activeIDs[ct.ID] = struct{}{}
	}
	c.cacheMu.Lock()
	for id := range c.startedCache {
		if _, ok := activeIDs[id]; !ok {
			delete(c.startedCache, id)
		}
	}
	c.cacheMu.Unlock()

	result := make([]ContainerInfo, 0, len(containers))
	for _, ct := range containers {
		name := ""
		if len(ct.Names) > 0 {
			name = strings.TrimPrefix(ct.Names[0], "/")
		}
		if name == "" {
			name = shortID(ct.ID)
		}

		stateStr := containerSummaryStateString(ct.State)
		startedAt := c.getContainerStartedAt(ctx, ct.ID, stateStr)

		ports := make([]PortInfo, 0, len(ct.Ports))
		for _, port := range ct.Ports {
			ipStr := ""
			if port.IP.IsValid() {
				ipStr = port.IP.String()
			}
			ports = append(ports, PortInfo{
				IP:          ipStr,
				PrivatePort: int(port.PrivatePort),
				PublicPort:  int(port.PublicPort),
				Type:        port.Type,
			})
		}

		repoDigests := imageDigests[ct.ImageID]
		platform := imagePlatforms[ct.ImageID]

		info := ContainerInfo{
			ID:           ct.ID,
			Name:         name,
			Image:        ct.Image,
			ImageID:      ct.ImageID,
			RepoTags:     imageTags[ct.ImageID],
			RepoDigests:  repoDigests,
			IsLocalImage: imageLocalFlags[ct.ImageID],
			Labels:       ct.Labels,
			State:        stateStr,
			Status:       ct.Status,
			Created:      ct.Created,
			StartedAt:    startedAt,
			Ports:        ports,
			ImageOS:      platform[0],
			ImageArch:    platform[1],
			ImageVariant: platform[2],
		}
		result = append(result, info)
	}
	return result, nil
}

func (c *Client) inspectImage(ctx context.Context, imageID string) (image.InspectResponse, error) {
	meta, err := c.inspectImageMeta(ctx, imageID)
	if err != nil {
		return image.InspectResponse{}, err
	}
	return meta.Inspect, nil
}

func (c *Client) inspectImageMeta(ctx context.Context, imageID string) (cachedImageMeta, error) {
	if cached, ok := c.getCachedImageMeta(imageID); ok {
		return cached, nil
	}

	res, err := c.docker.ImageInspect(ctx, imageID, client.ImageInspectWithManifests(true))
	if err == nil {
		return c.setCachedImageInspect(imageID, res.InspectResponse), nil
	}

	// 旧 daemon 不支持 manifests 查询时退回基础 inspect。
	res, err = c.docker.ImageInspect(ctx, imageID)
	if err != nil {
		return cachedImageMeta{}, err
	}
	return c.setCachedImageInspect(imageID, res.InspectResponse), nil
}

func comparableRepoDigests(img image.InspectResponse) []string {
	if len(img.RepoDigests) > 0 {
		return append([]string(nil), img.RepoDigests...)
	}

	digests := make([]string, 0, len(img.Manifests)+1)
	seen := make(map[string]struct{}, len(img.Manifests)+1)
	addDigest := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		digests = append(digests, value)
	}

	for _, manifest := range img.Manifests {
		addDigest(manifest.ID)
		addDigest(manifest.Descriptor.Digest.String())
	}
	if img.Descriptor != nil {
		addDigest(img.Descriptor.Digest.String())
	}

	return digests
}

func classifyLocalImage(img image.InspectResponse, comparableDigests []string) (local bool, needsParent bool) {
	if len(img.RepoDigests) > 0 {
		return false, false
	}
	if img.Identity != nil && len(img.Identity.Build) > 0 && len(img.Identity.Pull) == 0 {
		return true, false
	}
	if len(comparableDigests) > 0 {
		return false, false
	}
	if !hasNamedRepoTag(img.RepoTags) {
		return true, false
	}
	return false, true
}

func isLocalImage(img image.InspectResponse, parentID string, comparableDigests []string) bool {
	local, needsParent := classifyLocalImage(img, comparableDigests)
	if !needsParent {
		return local
	}
	return strings.TrimSpace(parentID) != ""
}

func hasNamedRepoTag(tags []string) bool {
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || tag == "<none>:<none>" {
			continue
		}
		return true
	}
	return false
}

// InspectContainer 返回容器的详细信息
func (c *Client) InspectContainer(ctx context.Context, id string) (container.InspectResponse, error) {
	res, err := c.docker.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return container.InspectResponse{}, err
	}
	return res.Container, nil
}

// StopContainer 停止容器（可选超时时间，单位秒）
func (c *Client) StopContainer(ctx context.Context, id string, timeoutSeconds int) error {
	var timeout *int
	if timeoutSeconds > 0 {
		t := timeoutSeconds
		timeout = &t
	}

	_, err := c.docker.ContainerStop(ctx, id, client.ContainerStopOptions{Timeout: timeout})
	if err == nil {
		c.InvalidateContainerCache(id)
	}
	return err
}

// RestartContainer 重启容器（可选超时时间，单位秒）
func (c *Client) RestartContainer(ctx context.Context, id string, timeoutSeconds int) error {
	var timeout *int
	if timeoutSeconds > 0 {
		t := timeoutSeconds
		timeout = &t
	}

	_, err := c.docker.ContainerRestart(ctx, id, client.ContainerRestartOptions{Timeout: timeout})
	if err == nil {
		c.InvalidateContainerCache(id)
	}
	return err
}

// RenameContainer 重命名容器
func (c *Client) RenameContainer(ctx context.Context, id string, newName string) error {
	_, err := c.docker.ContainerRename(ctx, id, client.ContainerRenameOptions{NewName: newName})
	return err
}

// CreateContainer 使用给定配置与名称创建容器
func (c *Client) CreateContainer(ctx context.Context, name string, cfg *container.Config, host *container.HostConfig, netCfg *network.NetworkingConfig) (string, error) {
	return c.CreateContainerWithPlatform(ctx, name, cfg, host, netCfg, nil)
}

// CreateContainerWithPlatform 使用给定配置、平台与名称创建容器
func (c *Client) CreateContainerWithPlatform(ctx context.Context, name string, cfg *container.Config, host *container.HostConfig, netCfg *network.NetworkingConfig, platform *ocispec.Platform) (string, error) {
	resp, err := c.docker.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:           cfg,
		HostConfig:       host,
		NetworkingConfig: netCfg,
		Platform:         platform,
		Name:             name,
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// ConnectContainerToNetwork 将指定容器连接到给定网络（用于多网络附加配置）
func (c *Client) ConnectContainerToNetwork(ctx context.Context, networkID, containerID string, endpoint *network.EndpointSettings) error {
	_, err := c.docker.NetworkConnect(ctx, networkID, client.NetworkConnectOptions{
		Container:      containerID,
		EndpointConfig: endpoint,
	})
	return err
}

// StartContainer 启动容器
func (c *Client) StartContainer(ctx context.Context, id string) error {
	_, err := c.docker.ContainerStart(ctx, id, client.ContainerStartOptions{})
	if err == nil {
		c.InvalidateContainerCache(id)
	}
	return err
}

// RemoveContainer 删除容器
func (c *Client) RemoveContainer(ctx context.Context, id string, force bool) error {
	_, err := c.docker.ContainerRemove(ctx, id, client.ContainerRemoveOptions{Force: force, RemoveVolumes: false, RemoveLinks: false})
	if err == nil {
		c.InvalidateContainerCache(id)
	}
	return err
}

// RemoveContainerWithVolumes 删除容器并清理关联的匿名卷
func (c *Client) RemoveContainerWithVolumes(ctx context.Context, id string, force bool) error {
	_, err := c.docker.ContainerRemove(ctx, id, client.ContainerRemoveOptions{
		Force:         force,
		RemoveVolumes: true,
		RemoveLinks:   false,
	})
	if err == nil {
		c.InvalidateContainerCache(id)
	}
	return err
}

// WaitContainerStopped 等待容器完全停止
func (c *Client) WaitContainerStopped(ctx context.Context, id string, maxWaitSeconds int) error {
	timeout := time.Duration(maxWaitSeconds) * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		res, err := c.docker.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errdefs.IsNotFound(err) {
				// 容器已不存在（例如 --rm 自动移除），认为已停止
				return nil
			}
			return err
		}

		if res.Container.State != nil && !res.Container.State.Running {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(300 * time.Millisecond):
			}
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}

	// 超时后强制杀死容器
	_, _ = c.docker.ContainerKill(ctx, id, client.ContainerKillOptions{Signal: "SIGKILL"})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(300 * time.Millisecond):
	}
	return nil
}

// PruneSystem 清理悬挂的文件系统、网络、镜像等
func (c *Client) PruneSystem(ctx context.Context) error {
	res, err := c.docker.VolumePrune(ctx, client.VolumePruneOptions{})
	logger.Logger.Info("VolumesPrune", zap.Any("res", res))
	if err != nil {
		return err
	}

	netRes, err := c.docker.NetworkPrune(ctx, client.NetworkPruneOptions{})
	logger.Logger.Info("NetworksPrune", zap.Any("res", netRes))
	if err != nil {
		return err
	}

	imgFilter := make(client.Filters).Add("dangling", "true")
	imgRes, err := c.docker.ImagePrune(ctx, client.ImagePruneOptions{Filters: imgFilter})
	logger.Logger.Info("ImagesPrune", zap.Any("res", imgRes))
	return err
}

// SafeRemoveImage 安全删除镜像（检查是否有其他容器使用）
func (c *Client) SafeRemoveImage(ctx context.Context, imageID string) error {
	if imageID == "" {
		return nil
	}

	listRes, err := c.docker.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return err
	}

	for _, ct := range listRes.Items {
		if ct.ImageID == imageID {
			return nil
		}
	}
	logger.Logger.Info("没有其他容器使用，可以安全删除镜像", zap.String("imageID", imageID))
	_, err = c.docker.ImageRemove(ctx, imageID, client.ImageRemoveOptions{Force: true})
	if err == nil {
		c.InvalidateImageCache()
	}
	return err
}

// SafeRemoveNetworks 安全删除自定义网络（检查是否有其他容器使用）
func (c *Client) SafeRemoveNetworks(ctx context.Context, networkIDs []string) error {
	for _, networkID := range networkIDs {
		if networkID == "" {
			continue
		}

		res, err := c.docker.NetworkInspect(ctx, networkID, client.NetworkInspectOptions{})
		if err != nil {
			continue
		}
		netInfo := res.Network

		if netInfo.Name == "bridge" || netInfo.Name == "host" || netInfo.Name == "none" {
			continue
		}

		if len(netInfo.Containers) > 0 {
			continue
		}

		_, _ = c.docker.NetworkRemove(ctx, networkID, client.NetworkRemoveOptions{})
	}
	return nil
}

// isHex64 检查卷名是否为 Docker 自动生成的 64 位十六进制匿名卷 ID
func isHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// ResolveMountVolumeName 解析卷挂载名称；若 MountPoint 未填充 Name，则尝试从标准 Docker 卷 _data 路径提取。
func ResolveMountVolumeName(m container.MountPoint) string {
	if m.Type != mount.TypeVolume {
		return ""
	}
	if m.Name != "" {
		return m.Name
	}
	if m.Source == "" {
		return ""
	}
	norm := strings.TrimSuffix(strings.ReplaceAll(m.Source, "\\", "/"), "/")
	if !strings.HasSuffix(norm, "/_data") {
		return ""
	}
	parent := strings.TrimSuffix(norm, "/_data")
	idx := strings.LastIndex(parent, "/")
	if idx < 0 {
		return ""
	}
	prefix, volName := parent[:idx], parent[idx+1:]
	if volName != "" && (prefix == "/volumes" || strings.HasSuffix(prefix, "/volumes")) {
		return volName
	}
	return ""
}

// isAnonymousVolume 准确判断卷是否为匿名卷，避免误删长名字的命名卷
func isAnonymousVolume(volName string, labels map[string]string) bool {
	if labels["com.docker.compose.volume"] != "" {
		return false
	}
	if _, ok := labels["com.docker.volume.anonymous"]; ok {
		return true
	}
	return isHex64(volName)
}

// SafeRemoveVolumes 安全删除匿名卷（检查是否有其他容器使用）
func (c *Client) SafeRemoveVolumes(ctx context.Context, volumeNames []string) error {
	if len(volumeNames) == 0 {
		return nil
	}

	// 只调用一次 ContainerList，直接从 ct.Mounts 收集正在使用的卷（避免 O(V*N) ContainerInspect）
	listRes, err := c.docker.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return err
	}
	inUse := make(map[string]struct{})
	for _, ct := range listRes.Items {
		for _, m := range ct.Mounts {
			if volName := ResolveMountVolumeName(m); volName != "" {
				inUse[volName] = struct{}{}
			}
		}
	}

	for _, volumeName := range volumeNames {
		if volumeName == "" {
			continue
		}
		if _, used := inUse[volumeName]; used {
			continue
		}

		volRes, err := c.docker.VolumeInspect(ctx, volumeName, client.VolumeInspectOptions{})
		if err != nil {
			continue
		}

		if !isAnonymousVolume(volRes.Volume.Name, volRes.Volume.Labels) {
			continue
		}

		_, _ = c.docker.VolumeRemove(ctx, volumeName, client.VolumeRemoveOptions{Force: false})
	}
	return nil
}

// ExportContainer 导出容器为tar格式的文件流
func (c *Client) ExportContainer(ctx context.Context, id string) (io.ReadCloser, error) {
	return c.docker.ContainerExport(ctx, id, client.ContainerExportOptions{})
}

// CleanupContainerResources 根据容器信息安全清理相关资源
func (c *Client) CleanupContainerResources(ctx context.Context, containerInfo container.InspectResponse) error {
	if containerInfo.Image != "" {
		_ = c.SafeRemoveImage(ctx, containerInfo.Image)
	}

	if len(containerInfo.Mounts) > 0 {
		var volumeNames []string
		for _, m := range containerInfo.Mounts {
			if volName := ResolveMountVolumeName(m); volName != "" {
				volumeNames = append(volumeNames, volName)
			}
		}
		_ = c.SafeRemoveVolumes(ctx, volumeNames)
	}

	return nil
}

// ContainerLogs 获取容器日志流
func (c *Client) ContainerLogs(ctx context.Context, containerID string, since string, timestamps bool, tail string, follow bool) (io.ReadCloser, error) {
	options := client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Since:      since,
		Timestamps: timestamps,
		Follow:     follow,
		Tail:       tail,
	}
	return c.docker.ContainerLogs(ctx, containerID, options)
}

// ContainerExecCreate 在容器中创建 exec 实例
func (c *Client) ContainerExecCreate(ctx context.Context, containerID string, config client.ExecCreateOptions) (client.ExecCreateResult, error) {
	return c.docker.ExecCreate(ctx, containerID, config)
}

// ContainerExecAttach 附加到 exec 实例
func (c *Client) ContainerExecAttach(ctx context.Context, execID string, config client.ExecAttachOptions) (client.ExecAttachResult, error) {
	return c.docker.ExecAttach(ctx, execID, config)
}

// ContainerExecResize 调整 exec 实例的终端大小
func (c *Client) ContainerExecResize(ctx context.Context, execID string, options client.ExecResizeOptions) error {
	_, err := c.docker.ExecResize(ctx, execID, options)
	return err
}

func (c *Client) ContainerStats(ctx context.Context, containerID string, stream bool) (client.ContainerStatsResult, error) {
	resp, err := c.docker.ContainerStats(ctx, containerID, client.ContainerStatsOptions{
		Stream:                stream,
		IncludePreviousSample: false,
	})
	if err != nil {
		return client.ContainerStatsResult{}, err
	}
	return resp, nil
}
