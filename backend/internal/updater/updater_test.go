package updater

import (
	"net/netip"
	"testing"

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
