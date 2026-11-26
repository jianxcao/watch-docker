package updater

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	dockerimage "github.com/moby/docker-image-spec/specs-go/v1"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestBuildNewContainerConfigsEntrypointLabelsVolumesAndNetworks(t *testing.T) {
	u := &Updater{}

	port80 := network.MustParsePort("80/tcp")
	port443 := network.MustParsePort("443/tcp")

	uctx := &updateContext{
		containerID: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		imageRef:    "nginx:latest",
		oldImage: &image.InspectResponse{
			Config: &dockerimage.DockerOCIImageConfig{
				ImageConfig: ocispec.ImageConfig{
					Entrypoint:   []string{"/docker-entrypoint.sh"},
					Cmd:          []string{"nginx", "-g", "daemon off;"},
					ExposedPorts: map[string]struct{}{"80/tcp": {}},
					Labels: map[string]string{
						"org.opencontainers.image.version": "1.25",
					},
				},
			},
		},
		oldInfo: container.InspectResponse{
			ID: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			Config: &container.Config{
				Image:      "nginx:latest",
				Entrypoint: nil, // 模拟 --entrypoint "" 清空镜像默认 Entrypoint
				Cmd:        []string{"nginx", "-g", "daemon off;"},
				ExposedPorts: network.PortSet{
					port80:  {},
					port443: {},
				},
				Labels: map[string]string{
					"org.opencontainers.image.version": "1.25",
					"com.docker.compose.project":       "demo",
					"watchdocker.skip":                 "false",
					"custom.label":                     "keep",
				},
			},
			HostConfig: &container.HostConfig{
				NetworkMode:  container.NetworkMode("app_net"),
				Binds:        []string{"C:\\data:/app:ro"},
				PortBindings: network.PortMap{port443: {{HostPort: "8443"}}},
			},
			Mounts: []container.MountPoint{
				{
					Type:        mount.TypeVolume,
					Name:        "anon_vol_1",
					Destination: "/data",
					RW:          true,
				},
				{
					Type:        mount.TypeVolume,
					Name:        "dup_vol",
					Destination: "/app",
					RW:          false,
				},
			},
			NetworkSettings: &container.NetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"bridge": {
						Aliases:    []string{"bad-bridge-alias"},
						Links:      []string{"db:db"},
						MacAddress: network.HardwareAddr{0x02, 0x42, 0xac, 0x11, 0x00, 0x02},
						IPAMConfig: &network.EndpointIPAMConfig{
							IPv4Address: netip.MustParseAddr("172.17.0.5"),
						},
					},
					"app_net": {
						Aliases:    []string{"web", "0123456789ab"},
						MacAddress: network.HardwareAddr{0x02, 0x42, 0xac, 0x1e, 0x00, 0x0a},
						IPAMConfig: &network.EndpointIPAMConfig{
							IPv4Address: netip.MustParseAddr("172.30.0.10"),
						},
					},
				},
			},
		},
	}

	newCfg, newHostCfg, netCfg, _, extra := u.buildNewContainerConfigs(uctx)

	if len(newCfg.Entrypoint) != 1 || newCfg.Entrypoint[0] != "" {
		t.Fatalf("expected --entrypoint \"\" override preserved, got %#v", newCfg.Entrypoint)
	}
	if len(newCfg.Cmd) == 0 {
		t.Fatalf("expected Cmd preserved when Entrypoint was overridden")
	}
	if _, ok := newCfg.Labels["org.opencontainers.image.version"]; ok {
		t.Fatalf("expected old image default label stripped")
	}
	if newCfg.Labels["com.docker.compose.project"] != "demo" || newCfg.Labels["watchdocker.skip"] != "false" || newCfg.Labels["custom.label"] != "keep" {
		t.Fatalf("expected compose/watchdocker/custom labels kept, got %#v", newCfg.Labels)
	}
	if _, ok := newCfg.ExposedPorts[port80]; ok {
		t.Fatalf("expected unbound image ExposedPort 80/tcp stripped")
	}
	if _, ok := newCfg.ExposedPorts[port443]; !ok {
		t.Fatalf("expected bound port 443/tcp retained")
	}
	if len(newHostCfg.Mounts) != 1 || newHostCfg.Mounts[0].Source != "anon_vol_1" {
		t.Fatalf("expected only /data anonymous volume migrated without duplicating /app bind, got %#v", newHostCfg.Mounts)
	}
	if string(newHostCfg.NetworkMode) != "app_net" {
		t.Fatalf("expected primary NetworkMode app_net preserved, got %q", newHostCfg.NetworkMode)
	}
	appEp := netCfg.EndpointsConfig["app_net"]
	if appEp == nil {
		t.Fatalf("expected app_net in primary EndpointsConfig")
	}
	if appEp.MacAddress.String() != "02:42:ac:1e:00:0a" {
		t.Fatalf("expected app_net MacAddress preserved, got %q", appEp.MacAddress.String())
	}
	bridgeEp := extra["bridge"]
	if bridgeEp == nil || len(bridgeEp.Aliases) != 0 || len(bridgeEp.Links) != 0 || bridgeEp.IPAMConfig != nil {
		t.Fatalf("expected bridge endpoint stripped of Aliases/Links/IPAMConfig, got %#v", bridgeEp)
	}
	if bridgeEp.MacAddress.String() != "02:42:ac:11:00:02" {
		t.Fatalf("expected bridge MacAddress preserved, got %q", bridgeEp.MacAddress.String())
	}
	// 验证克隆的 MacAddress 切片与旧结构隔离
	uctx.oldInfo.NetworkSettings.Networks["app_net"].MacAddress[0] = 0xff
	if appEp.MacAddress.String() != "02:42:ac:1e:00:0a" {
		t.Fatalf("expected cloned MacAddress isolated from source slice mutation, got %q", appEp.MacAddress.String())
	}
}

func TestBuildNewContainerConfigsResolvesPrimaryNetworkByID(t *testing.T) {
	u := &Updater{}
	netID := "feedface1234567890abcdef1234567890abcdef1234567890abcdef12345678"
	uctx := &updateContext{
		containerID: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		imageRef:    "nginx:latest",
		oldInfo: container.InspectResponse{
			Config: &container.Config{Image: "nginx:latest"},
			HostConfig: &container.HostConfig{
				NetworkMode: container.NetworkMode(netID[:12]),
			},
			NetworkSettings: &container.NetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"aaa_secondary": {NetworkID: "1111111111111111111111111111111111111111111111111111111111111111", MacAddress: network.HardwareAddr{}},
					"bbb_nil":       nil,
					"zzz_primary":   {NetworkID: netID},
				},
			},
		},
	}

	_, newHostCfg, netCfg, _, extra := u.buildNewContainerConfigs(uctx)
	if string(newHostCfg.NetworkMode) != "zzz_primary" {
		t.Fatalf("expected primary NetworkMode resolved to zzz_primary by NetworkID, got %q", newHostCfg.NetworkMode)
	}
	if _, ok := netCfg.EndpointsConfig["zzz_primary"]; !ok {
		t.Fatalf("expected zzz_primary in EndpointsConfig, got %#v", netCfg.EndpointsConfig)
	}
	secEp := extra["aaa_secondary"]
	if secEp == nil || secEp.MacAddress != nil {
		t.Fatalf("expected empty MacAddress normalized to nil, got %#v", secEp)
	}
	if extra["bbb_nil"] == nil || extra["bbb_nil"].MacAddress != nil {
		t.Fatalf("expected nil endpoint sanitized safely, got %#v", extra["bbb_nil"])
	}
}

func TestBuildNewContainerConfigsDestinationOnlyBind(t *testing.T) {
	u := &Updater{}

	t.Run("rewrites destination-only bind to existing volume and avoids duplicate mount", func(t *testing.T) {
		uctx := &updateContext{oldInfo: container.InspectResponse{
			Config:     &container.Config{},
			HostConfig: &container.HostConfig{Binds: []string{"/data", "logs:/logs:ro"}},
			Mounts: []container.MountPoint{
				{Type: mount.TypeVolume, Name: "old-anon", Destination: "/data", RW: true},
				{Type: mount.TypeVolume, Name: "logs", Destination: "/logs", RW: false},
			},
		}}

		_, newHostCfg, _, _, _ := u.buildNewContainerConfigs(uctx)

		if len(newHostCfg.Binds) != 2 || newHostCfg.Binds[0] != "old-anon:/data" || newHostCfg.Binds[1] != "logs:/logs:ro" {
			t.Fatalf("unexpected Binds: %#v", newHostCfg.Binds)
		}
		if len(newHostCfg.Mounts) != 0 {
			t.Fatalf("expected no duplicate structured Mounts, got %#v", newHostCfg.Mounts)
		}
		if uctx.oldInfo.HostConfig.Binds[0] != "/data" {
			t.Fatalf("expected source HostConfig.Binds untouched, got %#v", uctx.oldInfo.HostConfig.Binds)
		}
	})

	t.Run("preserves read-only flag and missing or autoremove binds", func(t *testing.T) {
		uctx := &updateContext{oldInfo: container.InspectResponse{
			Config:     &container.Config{},
			HostConfig: &container.HostConfig{Binds: []string{"/rodata", "/unbacked"}},
			Mounts: []container.MountPoint{
				{Type: mount.TypeVolume, Name: "old-ro", Destination: "/rodata", RW: false},
			},
		}}

		_, newHostCfg, _, _, _ := u.buildNewContainerConfigs(uctx)
		if len(newHostCfg.Binds) != 2 || newHostCfg.Binds[0] != "old-ro:/rodata:ro" || newHostCfg.Binds[1] != "/unbacked" {
			t.Fatalf("unexpected Binds for ro/unbacked: %#v", newHostCfg.Binds)
		}

		autoRm := &updateContext{oldInfo: container.InspectResponse{
			Config:     &container.Config{},
			HostConfig: &container.HostConfig{AutoRemove: true, Binds: []string{"/ephemeral"}},
			Mounts: []container.MountPoint{
				{Type: mount.TypeVolume, Name: "ephem-vol", Destination: "/ephemeral", RW: true},
			},
		}}
		_, autoHostCfg, _, _, _ := u.buildNewContainerConfigs(autoRm)
		if len(autoHostCfg.Binds) != 1 || autoHostCfg.Binds[0] != "/ephemeral" || len(autoHostCfg.Mounts) != 0 {
			t.Fatalf("expected AutoRemove bind preserved without migration, got binds=%#v mounts=%#v", autoHostCfg.Binds, autoHostCfg.Mounts)
		}
	})

	t.Run("extracts volume name from _data Source when Engine omits MountPoint Name", func(t *testing.T) {
		uctx := &updateContext{oldInfo: container.InspectResponse{
			Config:     &container.Config{},
			HostConfig: &container.HostConfig{Binds: []string{"/data", "/unbacked"}},
			Mounts: []container.MountPoint{
				{
					Type:        mount.TypeVolume,
					Name:        "",
					Source:      "/var/lib/docker/volumes/aab36938/_data",
					Destination: "/data",
					RW:          true,
				},
				{
					Type:        mount.TypeVolume,
					Name:        "",
					Source:      "",
					Destination: "/unbacked",
					RW:          true,
				},
			},
		}}

		_, newHostCfg, _, _, _ := u.buildNewContainerConfigs(uctx)
		if len(newHostCfg.Binds) != 2 || newHostCfg.Binds[0] != "aab36938:/data" || newHostCfg.Binds[1] != "/unbacked" {
			t.Fatalf("expected _data Source rewritten to aab36938:/data while leaving /unbacked, got %#v", newHostCfg.Binds)
		}
		if len(newHostCfg.Mounts) != 0 {
			t.Fatalf("expected zero duplicate Mounts, got %#v", newHostCfg.Mounts)
		}
	})

	t.Run("normalizes unclean POSIX destination-only binds against inspect destination", func(t *testing.T) {
		for _, rawBind := range []string{"/var/./data", "/var//data", "/var/data/"} {
			for _, rw := range []bool{true, false} {
				uctx := &updateContext{oldInfo: container.InspectResponse{
					Config:     &container.Config{},
					HostConfig: &container.HostConfig{Binds: []string{rawBind}},
					Mounts: []container.MountPoint{
						{
							Type:        mount.TypeVolume,
							Name:        "original",
							Destination: "/var/data",
							RW:          rw,
						},
					},
				}}

				_, newHostCfg, _, _, _ := u.buildNewContainerConfigs(uctx)
				wantBind := "original:/var/data"
				if !rw {
					wantBind += ":ro"
				}
				if len(newHostCfg.Binds) != 1 || newHostCfg.Binds[0] != wantBind {
					t.Fatalf("rawBind=%q rw=%v: got Binds=%#v, want [%q]", rawBind, rw, newHostCfg.Binds, wantBind)
				}
				if len(newHostCfg.Mounts) != 0 {
					t.Fatalf("rawBind=%q rw=%v: expected no duplicate Mounts, got %#v", rawBind, rw, newHostCfg.Mounts)
				}
				if uctx.oldInfo.HostConfig.Binds[0] != rawBind {
					t.Fatalf("expected old HostConfig.Binds untouched, got %#v", uctx.oldInfo.HostConfig.Binds)
				}
			}
		}

		// 带来源的未规范化 POSIX 绑定也应去重，不再追加重复挂载
		namedUnclean := &updateContext{oldInfo: container.InspectResponse{
			Config:     &container.Config{},
			HostConfig: &container.HostConfig{Binds: []string{"myvol:/var/./data:ro"}},
			Mounts: []container.MountPoint{
				{
					Type:        mount.TypeVolume,
					Name:        "myvol",
					Destination: "/var/data",
					RW:          false,
				},
			},
		}}
		_, namedHostCfg, _, _, _ := u.buildNewContainerConfigs(namedUnclean)
		if len(namedHostCfg.Binds) != 1 || namedHostCfg.Binds[0] != "myvol:/var/./data:ro" || len(namedHostCfg.Mounts) != 0 {
			t.Fatalf("expected named unclean bind deduplicated, got binds=%#v mounts=%#v", namedHostCfg.Binds, namedHostCfg.Mounts)
		}
	})

	t.Run("migrates HostConfig.Mounts anonymous volume without emptying source", func(t *testing.T) {
		uctx := &updateContext{oldInfo: container.InspectResponse{
			Config: &container.Config{},
			HostConfig: &container.HostConfig{
				Mounts: []mount.Mount{
					{Type: mount.TypeVolume, Source: "", Target: "/data"},
				},
			},
			Mounts: []container.MountPoint{
				{Type: mount.TypeVolume, Name: "anon_mount_vol", Destination: "/data", RW: false},
			},
		}}

		_, newHostCfg, _, _, _ := u.buildNewContainerConfigs(uctx)
		if len(newHostCfg.Mounts) != 1 || newHostCfg.Mounts[0].Source != "anon_mount_vol" || !newHostCfg.Mounts[0].ReadOnly {
			t.Fatalf("expected structured mount rewritten with Source=anon_mount_vol ReadOnly=true, got %#v", newHostCfg.Mounts)
		}
		if uctx.oldInfo.HostConfig.Mounts[0].Source != "" {
			t.Fatalf("expected source HostConfig.Mounts untouched, got %#v", uctx.oldInfo.HostConfig.Mounts)
		}
	})

	t.Run("normalizes Windows destination-only binds and source:target binds", func(t *testing.T) {
		uctx := &updateContext{oldInfo: container.InspectResponse{
			Config: &container.Config{},
			HostConfig: &container.HostConfig{
				Binds: []string{
					`C:\ProgramData`,
					`C:\host:D:\app:ro`,
					`MyVol:E:\AppData:ro`,
					`D:`,
					`C:\Host:F::ro`,
				},
			},
			Mounts: []container.MountPoint{
				{Type: mount.TypeVolume, Name: "win_anon", Destination: `c:\programdata`, RW: true},
				{Type: mount.TypeVolume, Name: "myvol", Destination: `e:\appdata`, RW: false},
				{Type: mount.TypeVolume, Name: "drive_anon", Destination: `d:`, RW: true},
			},
		}}

		_, newHostCfg, _, _, _ := u.buildNewContainerConfigs(uctx)
		if len(newHostCfg.Binds) != 5 || newHostCfg.Binds[0] != `win_anon:c:\programdata` || newHostCfg.Binds[3] != `drive_anon:d:` {
			t.Fatalf("expected Windows dest binds rewritten cleanly, got %#v", newHostCfg.Binds)
		}
		if len(newHostCfg.Mounts) != 0 {
			t.Fatalf("expected zero duplicate Mounts for mixed-case/drive-only Windows binds, got %#v", newHostCfg.Mounts)
		}
	})
}

func TestBuildNewContainerConfigsPreservesUser(t *testing.T) {
	u := &Updater{}
	cases := []struct {
		name          string
		containerUser string
		imageUser     string
		wantUser      string
	}{
		{
			name:          "numeric uid:gid matching image user is preserved",
			containerUser: "1000:1000",
			imageUser:     "1000:1000",
			wantUser:      "1000:1000",
		},
		{
			name:          "named user matching image user is preserved",
			containerUser: "app",
			imageUser:     "app",
			wantUser:      "app",
		},
		{
			name:          "different container user is preserved",
			containerUser: "2000",
			imageUser:     "1000",
			wantUser:      "2000",
		},
		{
			name:          "empty user delegates default to new image",
			containerUser: "",
			imageUser:     "",
			wantUser:      "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uctx := &updateContext{
				imageRef: "example/app:v2",
				oldImage: &image.InspectResponse{
					Config: &dockerimage.DockerOCIImageConfig{
						ImageConfig: ocispec.ImageConfig{
							User: tc.imageUser,
						},
					},
				},
				oldInfo: container.InspectResponse{
					Config: &container.Config{
						Image: "example/app:v1",
						User:  tc.containerUser,
					},
				},
			}

			newCfg, _, _, _, _ := u.buildNewContainerConfigs(uctx)
			if newCfg.User != tc.wantUser {
				t.Fatalf("containerUser=%q imageUser=%q: got newCfg.User=%q, want %q",
					tc.containerUser, tc.imageUser, newCfg.User, tc.wantUser)
			}
		})
	}
}

func TestBuildNewContainerConfigsPreservesExistingEnvIncludingEmptyValue(t *testing.T) {
	u := &Updater{}
	uctx := &updateContext{
		imageRef: "example/app:v2",
		oldImage: &image.InspectResponse{
			Config: &dockerimage.DockerOCIImageConfig{
				ImageConfig: ocispec.ImageConfig{
					Env: []string{
						"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
						"MODE=production",
						"TOKEN=",
					},
				},
			},
		},
		oldInfo: container.InspectResponse{
			Config: &container.Config{
				Image: "example/app:v1",
				Env: []string{
					"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
					"MODE=production",
					"TOKEN=",
					"CUSTOM=1",
				},
			},
		},
	}

	newCfg, _, _, _, _ := u.buildNewContainerConfigs(uctx)
	for _, want := range []string{"MODE=production", "TOKEN=", "CUSTOM=1"} {
		if !slices.Contains(newCfg.Env, want) {
			t.Fatalf("expected newCfg.Env to preserve %q, got %#v", want, newCfg.Env)
		}
	}

	uctx.oldInfo.Config.Env[0] = "MUTATED=1"
	if slices.Contains(newCfg.Env, "MUTATED=1") {
		t.Fatalf("expected newCfg.Env cloned independently from oldInfo.Config.Env")
	}
}

func buildDockerSaveArchive(t *testing.T, repoTag string, env []string, arch string) []byte {
	t.Helper()

	var layerBuf bytes.Buffer
	ltw := tar.NewWriter(&layerBuf)
	placeholder := []byte(repoTag + "\n")
	if err := ltw.WriteHeader(&tar.Header{
		Name: "etc/wd-env-marker",
		Mode: 0644,
		Size: int64(len(placeholder)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ltw.Write(placeholder); err != nil {
		t.Fatal(err)
	}
	if err := ltw.Close(); err != nil {
		t.Fatal(err)
	}
	layerBytes := layerBuf.Bytes()
	layerSum := sha256.Sum256(layerBytes)
	layerDiffID := "sha256:" + hex.EncodeToString(layerSum[:])
	layerEntry := "layer/layer.tar"

	imgCfg := map[string]any{
		"architecture": arch,
		"os":           "linux",
		"config": map[string]any{
			"Env": env,
			"Cmd": []string{"/bin/sh"},
		},
		"rootfs": map[string]any{
			"type":     "layers",
			"diff_ids": []string{layerDiffID},
		},
	}
	cfgBytes, err := json.Marshal(imgCfg)
	if err != nil {
		t.Fatal(err)
	}
	cfgSum := sha256.Sum256(cfgBytes)
	cfgName := hex.EncodeToString(cfgSum[:]) + ".json"

	manifestBytes, err := json.Marshal([]map[string]any{
		{
			"Config":   cfgName,
			"RepoTags": []string{repoTag},
			"Layers":   []string{layerEntry},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var archiveBuf bytes.Buffer
	atw := tar.NewWriter(&archiveBuf)
	addFile := func(name string, data []byte) {
		if err := atw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(data)),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := atw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	addFile(cfgName, cfgBytes)
	addFile(layerEntry, layerBytes)
	addFile("manifest.json", manifestBytes)
	if err := atw.Close(); err != nil {
		t.Fatal(err)
	}
	return archiveBuf.Bytes()
}

func TestUpdateContainerPreservesExistingEnvAndMergesNewImageEnv(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dcli, err := dockercli.New(ctx, "")
	if err != nil {
		t.Skipf("跳过 Docker 集成测试：无法连接 Docker 客户端: %v", err)
	}
	defer dcli.Close()

	ver, err := dcli.GetVersion(ctx)
	if err != nil {
		t.Skipf("跳过 Docker 集成测试：Docker daemon 不可用: %v", err)
	}
	arch := ver.Arch
	if arch == "" {
		arch = runtime.GOARCH
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	v1Ref := fmt.Sprintf("wd-env-verify-%s:v1", suffix)
	v2Ref := fmt.Sprintf("wd-env-verify-%s:v2", suffix)
	containerName := fmt.Sprintf("wd-env-verify-ctr-%s", suffix)

	v1Tar := buildDockerSaveArchive(t, v1Ref, []string{"MODE=production", "TOKEN="}, arch)
	v2Tar := buildDockerSaveArchive(t, v2Ref, []string{"MODE=development", "TOKEN=default", "NEW_FLAG=1"}, arch)

	if err := dcli.LoadImage(ctx, bytes.NewReader(v1Tar)); err != nil {
		t.Fatalf("load v1 test image: %v", err)
	}
	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanCancel()
		_ = dcli.RemoveImage(cleanCtx, v1Ref, true, true)
	})

	if err := dcli.LoadImage(ctx, bytes.NewReader(v2Tar)); err != nil {
		t.Fatalf("load v2 test image: %v", err)
	}
	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanCancel()
		_ = dcli.RemoveImage(cleanCtx, v2Ref, true, true)
	})

	oldID, err := dcli.CreateContainer(ctx, containerName, &container.Config{
		Image: v1Ref,
		Env:   []string{"MODE=production", "TOKEN="},
	}, &container.HostConfig{}, nil)
	if err != nil {
		t.Fatalf("create v1 container: %v", err)
	}
	var newID string
	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanCancel()
		if newID != "" {
			_ = dcli.RemoveContainer(cleanCtx, newID, true)
		}
		_ = dcli.RemoveContainer(cleanCtx, oldID, true)
	})

	u := New(dcli)
	uctx := &updateContext{
		containerID: oldID,
		imageRef:    v2Ref,
	}
	autoRemoved, err := u.prepareOldContainer(ctx, uctx)
	if err != nil || autoRemoved {
		t.Fatalf("prepareOldContainer failed: autoRemoved=%v err=%v", autoRemoved, err)
	}
	if err := u.createContainerWithRetry(ctx, uctx); err != nil {
		u.rollbackOnCreateFailure(ctx, uctx)
		t.Fatalf("createContainerWithRetry failed: %v", err)
	}
	newID = uctx.newID
	u.finalCleanup(ctx, uctx)

	inspected, err := dcli.InspectContainer(ctx, newID)
	if err != nil {
		t.Fatalf("inspect replaced container: %v", err)
	}
	if inspected.Config == nil {
		t.Fatalf("expected replaced container Config != nil")
	}

	gotEnv := inspected.Config.Env
	for _, want := range []string{"MODE=production", "TOKEN=", "NEW_FLAG=1"} {
		if !slices.Contains(gotEnv, want) {
			t.Fatalf("expected replaced container Env to contain %q, got %#v", want, gotEnv)
		}
	}
	for _, unwanted := range []string{"MODE=development", "TOKEN=default"} {
		if slices.Contains(gotEnv, unwanted) {
			t.Fatalf("expected replaced container Env NOT to revert to new image default %q, got %#v", unwanted, gotEnv)
		}
	}
	if gotName := strings.TrimPrefix(inspected.Name, "/"); gotName != containerName {
		t.Fatalf("expected replaced container name %q, got %q", containerName, gotName)
	}
}
