package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestHandleCreateContainerRollbackAcrossCancellationAndErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const targetID = "cid-rollback-123"

	type deleteRecord struct {
		id    string
		force string
	}

	var mu sync.Mutex
	var activeNames = make(map[string]string) // name -> containerID
	var deleted []deleteRecord
	var failNetConnect bool
	var failDelete bool
	var connectReachedCh chan struct{}
	var cancelDoneCh chan struct{}

	mockDocker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/events"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/images/json"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"Id": "sha256:abc", "RepoTags": []string{"alpine:latest"}},
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/containers/create"):
			name := r.URL.Query().Get("name")
			mu.Lock()
			if existing, exists := activeNames[name]; exists {
				mu.Unlock()
				http.Error(w, "Conflict: name already in use by "+existing, http.StatusConflict)
				return
			}
			activeNames[name] = targetID
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": targetID})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/networks/") && strings.HasSuffix(r.URL.Path, "/connect"):
			if strings.Contains(r.URL.Path, "/networks/net-b/connect") {
				if connectReachedCh != nil {
					close(connectReachedCh)
					<-cancelDoneCh
				}
				mu.Lock()
				shouldFail := failNetConnect
				mu.Unlock()
				if shouldFail {
					http.Error(w, "network net-b failure", http.StatusInternalServerError)
					return
				}
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/containers/"):
			id := strings.TrimPrefix(r.URL.Path[strings.LastIndex(r.URL.Path, "/containers/")+len("/containers/"):], "/")
			mu.Lock()
			shouldFailDel := failDelete
			if !shouldFailDel {
				for k, v := range activeNames {
					if v == id {
						delete(activeNames, k)
					}
				}
			}
			deleted = append(deleted, deleteRecord{id: id, force: r.URL.Query().Get("force")})
			mu.Unlock()
			if shouldFailDel {
				http.Error(w, "delete permission denied", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockDocker.Close()

	cli, err := dockercli.New(context.Background(), mockDocker.URL)
	if err != nil {
		t.Fatalf("create dockercli: %v", err)
	}
	defer cli.Close()

	makePayload := func() []byte {
		body, _ := json.Marshal(map[string]any{
			"name":  "multi-net-demo",
			"image": "alpine:latest",
			"networkConfig": map[string]any{
				"endpointsConfig": map[string]any{
					"net-a": map[string]any{},
					"net-b": map[string]any{},
				},
			},
		})
		return body
	}

	// 场景 1: 父请求在第二个网络连接期间被取消，仍能触发回滚删除并允许同名再次创建
	core, logs := observer.New(zap.ErrorLevel)
	srv := &Server{
		docker: cli,
		logger: zap.New(core),
	}
	r := gin.New()
	r.POST("/containers/create", srv.handleCreateContainer())

	mu.Lock()
	deleted = nil
	activeNames = make(map[string]string)
	failNetConnect = true
	failDelete = false
	mu.Unlock()
	connectReachedCh = make(chan struct{})
	cancelDoneCh = make(chan struct{})

	reqCtx, cancelReq := context.WithCancel(context.Background())
	go func() {
		<-connectReachedCh
		cancelReq()
		close(cancelDoneCh)
	}()

	req := httptest.NewRequest(http.MethodPost, "/containers/create", bytes.NewReader(makePayload())).WithContext(reqCtx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	mu.Lock()
	delCount := len(deleted)
	delCopy := append([]deleteRecord(nil), deleted...)
	mu.Unlock()
	if delCount != 1 || delCopy[0].id != targetID || (delCopy[0].force != "1" && delCopy[0].force != "true") {
		t.Fatalf("expected 1 force DELETE for %s after parent cancel, got %#v", targetID, delCopy)
	}

	// 场景 2: 回滚完成后同名再次创建且网络全部成功，DELETE 次数保持不变
	connectReachedCh = nil
	cancelDoneCh = nil
	mu.Lock()
	failNetConnect = false
	deleted = nil
	mu.Unlock()

	req2 := httptest.NewRequest(http.MethodPost, "/containers/create", bytes.NewReader(makePayload()))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)

	var res2 BaseRes
	if err := json.Unmarshal(rec2.Body.Bytes(), &res2); err != nil || res2.Code != 0 {
		t.Fatalf("expected second create with same name to succeed, status=%d body=%s err=%v", rec2.Code, rec2.Body.String(), err)
	}
	mu.Lock()
	if len(deleted) != 0 {
		t.Fatalf("expected 0 DELETE calls on successful create, got %#v", deleted)
	}
	mu.Unlock()

	// 场景 3: 删除本身失败时，日志同时包含原始网络连接失败与回滚失败原因，且响应保留网络连接错误
	mu.Lock()
	activeNames = make(map[string]string)
	failNetConnect = true
	failDelete = true
	deleted = nil
	mu.Unlock()

	req3 := httptest.NewRequest(http.MethodPost, "/containers/create", bytes.NewReader(makePayload()))
	req3.Header.Set("Content-Type", "application/json")
	rec3 := httptest.NewRecorder()
	r.ServeHTTP(rec3, req3)

	var res3 BaseRes
	if err := json.Unmarshal(rec3.Body.Bytes(), &res3); err != nil || res3.Code == 0 || !strings.Contains(res3.Msg, "net-b") {
		t.Fatalf("expected original network connect error preserved in response, got %s", rec3.Body.String())
	}
	var sawConnectErr, sawRollbackErr bool
	for _, entry := range logs.All() {
		if entry.Message == "connect extra network failed" {
			sawConnectErr = true
		}
		if entry.Message == "创建容器后回滚失败" {
			sawRollbackErr = true
		}
	}
	if !sawConnectErr || !sawRollbackErr {
		t.Fatalf("expected both connect and rollback errors logged, connect=%v rollback=%v", sawConnectErr, sawRollbackErr)
	}
}

func TestHandleCreateContainerRespectsPrimaryNetwork(t *testing.T) {
	gin.SetMode(gin.TestMode)

	type createRawBody struct {
		HostConfig struct {
			NetworkMode string `json:"NetworkMode"`
		} `json:"HostConfig"`
		NetworkingConfig struct {
			EndpointsConfig map[string]struct {
				Aliases   []string `json:"Aliases"`
				NetworkID string   `json:"NetworkID"`
			} `json:"EndpointsConfig"`
		} `json:"NetworkingConfig"`
	}
	type connectCall struct {
		Network string
		Aliases []string
	}

	var mu sync.Mutex
	var lastCreate *createRawBody
	var connects []connectCall

	mockDocker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/events"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/images/json"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"Id": "sha256:abc", "RepoTags": []string{"alpine:latest"}},
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/containers/create"):
			var body createRawBody
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			lastCreate = &body
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": "cid-net-test"})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/networks/") && strings.HasSuffix(r.URL.Path, "/connect"):
			trimmed := strings.TrimPrefix(r.URL.Path[strings.Index(r.URL.Path, "/networks/")+len("/networks/"):], "/")
			netName := strings.TrimSuffix(trimmed, "/connect")
			var reqBody struct {
				EndpointConfig *struct {
					Aliases []string `json:"Aliases"`
				} `json:"EndpointConfig"`
			}
			_ = json.NewDecoder(r.Body).Decode(&reqBody)
			var aliases []string
			if reqBody.EndpointConfig != nil {
				aliases = reqBody.EndpointConfig.Aliases
			}
			mu.Lock()
			connects = append(connects, connectCall{Network: netName, Aliases: aliases})
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockDocker.Close()

	cli, err := dockercli.New(context.Background(), mockDocker.URL)
	if err != nil {
		t.Fatalf("create dockercli: %v", err)
	}
	defer cli.Close()

	srv := &Server{
		docker: cli,
		logger: zap.NewNop(),
	}
	r := gin.New()
	r.POST("/containers/create", srv.handleCreateContainer())

	doReq := func(payload map[string]any) BaseRes {
		mu.Lock()
		lastCreate = nil
		connects = nil
		mu.Unlock()
		b, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/containers/create", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		var res BaseRes
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatalf("decode response failed: %v, body=%s", err, rec.Body.String())
		}
		return res
	}

	zNetID := "feedface1234567890abcdef1234567890abcdef1234567890abcdef12345678"
	aNetID := "1111111111111111111111111111111111111111111111111111111111111111"

	// 1. 显式按名称指定主网络 z_primary，Create 只带 z_primary，Connect 只附加 a_extra
	res := doReq(map[string]any{
		"image":       "alpine:latest",
		"networkMode": "z_primary",
		"networkConfig": map[string]any{
			"endpointsConfig": map[string]any{
				"a_extra":   map[string]any{"aliases": []string{"extra"}},
				"z_primary": map[string]any{"aliases": []string{"primary"}},
			},
		},
	})
	if res.Code != 0 {
		t.Fatalf("expected success, got %+v", res)
	}
	if lastCreate == nil || lastCreate.HostConfig.NetworkMode != "z_primary" ||
		len(lastCreate.NetworkingConfig.EndpointsConfig) != 1 ||
		len(lastCreate.NetworkingConfig.EndpointsConfig["z_primary"].Aliases) != 1 ||
		lastCreate.NetworkingConfig.EndpointsConfig["z_primary"].Aliases[0] != "primary" {
		t.Fatalf("unexpected Create payload for explicit z_primary: %#v", lastCreate)
	}
	if len(connects) != 1 || connects[0].Network != "a_extra" || len(connects[0].Aliases) != 1 || connects[0].Aliases[0] != "extra" {
		t.Fatalf("unexpected Connect calls: %#v", connects)
	}

	// 2. 通过完整 NetworkID 与唯一短前缀匹配 z_primary
	for _, mode := range []string{zNetID, zNetID[:12]} {
		res = doReq(map[string]any{
			"image":       "alpine:latest",
			"networkMode": mode,
			"networkConfig": map[string]any{
				"endpointsConfig": map[string]any{
					"a_extra":   map[string]any{"networkID": aNetID, "aliases": []string{"extra"}},
					"z_primary": map[string]any{"networkID": zNetID, "aliases": []string{"primary"}},
				},
			},
		})
		if res.Code != 0 {
			t.Fatalf("mode=%q expected success, got %+v", mode, res)
		}
		if lastCreate.HostConfig.NetworkMode != "z_primary" || lastCreate.NetworkingConfig.EndpointsConfig["z_primary"].Aliases[0] != "primary" {
			t.Fatalf("mode=%q expected Create z_primary, got %#v", mode, lastCreate)
		}
		if len(connects) != 1 || connects[0].Network != "a_extra" {
			t.Fatalf("mode=%q expected Connect a_extra, got %#v", mode, connects)
		}
	}

	// 3. 省略 networkMode 时沿用排序首项 a_extra
	res = doReq(map[string]any{
		"image": "alpine:latest",
		"networkConfig": map[string]any{
			"endpointsConfig": map[string]any{
				"a_extra":   map[string]any{"aliases": []string{"extra"}},
				"z_primary": map[string]any{"aliases": []string{"primary"}},
			},
		},
	})
	if res.Code != 0 || lastCreate.HostConfig.NetworkMode != "a_extra" || len(connects) != 1 || connects[0].Network != "z_primary" {
		t.Fatalf("omitted networkMode mismatch: res=%+v create=%#v connects=%#v", res, lastCreate, connects)
	}

	// 4. 显式指定不在 endpointsConfig 中的主网络时，保留主网络并附加端点表中所有网络
	res = doReq(map[string]any{
		"image":       "alpine:latest",
		"networkMode": "bridge",
		"networkConfig": map[string]any{
			"endpointsConfig": map[string]any{
				"a_extra": map[string]any{"aliases": []string{"extra"}},
			},
		},
	})
	if res.Code != 0 || lastCreate.HostConfig.NetworkMode != "bridge" || len(lastCreate.NetworkingConfig.EndpointsConfig) != 0 {
		t.Fatalf("explicit bridge without endpoint mismatch: res=%+v create=%#v", res, lastCreate)
	}
	if len(connects) != 1 || connects[0].Network != "a_extra" || len(connects[0].Aliases) != 1 || connects[0].Aliases[0] != "extra" {
		t.Fatalf("expected extra network connected on explicit bridge, got %#v", connects)
	}

	// 5. 多个端点共享同一个 NetworkID 前缀时返回 CodeBadRequest，且不调用 Create
	res = doReq(map[string]any{
		"image":       "alpine:latest",
		"networkMode": "abcd",
		"networkConfig": map[string]any{
			"endpointsConfig": map[string]any{
				"net1": map[string]any{"networkID": "abcd1111"},
				"net2": map[string]any{"networkID": "abcd2222"},
			},
		},
	})
	if res.Code != CodeBadRequest || lastCreate != nil {
		t.Fatalf("expected CodeBadRequest and no Create call on ambiguous prefix, got res=%+v create=%#v", res, lastCreate)
	}

	// 6. 特殊模式 host 保持 NetworkMode 不被 endpointsConfig 覆盖且不附加端点
	res = doReq(map[string]any{
		"image":       "alpine:latest",
		"networkMode": "host",
		"networkConfig": map[string]any{
			"endpointsConfig": map[string]any{
				"a_extra": map[string]any{"aliases": []string{"extra"}},
			},
		},
	})
	if res.Code != 0 || lastCreate.HostConfig.NetworkMode != "host" || len(lastCreate.NetworkingConfig.EndpointsConfig) != 0 || len(connects) != 0 {
		t.Fatalf("host mode mismatch: res=%+v create=%#v connects=%#v", res, lastCreate, connects)
	}

	// 7. networkMode: "default" 且含 "bridge" 端点时应归一化到 bridge 而非重复 Connect
	res = doReq(map[string]any{
		"image":       "alpine:latest",
		"networkMode": "default",
		"networkConfig": map[string]any{
			"endpointsConfig": map[string]any{
				"bridge": map[string]any{"macAddress": "02:42:ac:11:00:05"},
			},
		},
	})
	if res.Code != 0 || lastCreate.HostConfig.NetworkMode != "bridge" || len(lastCreate.NetworkingConfig.EndpointsConfig) != 1 || len(connects) != 0 {
		t.Fatalf("default->bridge mismatch: res=%+v create=%#v connects=%#v", res, lastCreate, connects)
	}

	// 8. 短名称或非十六进制主网络名（如 "db"）不被当作 NetworkID 前缀劫持
	res = doReq(map[string]any{
		"image":       "alpine:latest",
		"networkMode": "db",
		"networkConfig": map[string]any{
			"endpointsConfig": map[string]any{
				"app_net": map[string]any{"networkID": "db8a9f1234567890abcdef1234567890abcdef1234567890abcdef1234567890", "aliases": []string{"app"}},
			},
		},
	})
	if res.Code != 0 || lastCreate.HostConfig.NetworkMode != "db" || len(lastCreate.NetworkingConfig.EndpointsConfig) != 0 || len(connects) != 1 || connects[0].Network != "app_net" {
		t.Fatalf("short hex network name db mismatch: res=%+v create=%#v connects=%#v", res, lastCreate, connects)
	}

	// 9. 首尾带空格的特殊模式 " host " 应先规范化再识别为 host 模式
	res = doReq(map[string]any{
		"image":       "alpine:latest",
		"networkMode": " host ",
		"networkConfig": map[string]any{
			"endpointsConfig": map[string]any{
				"a_extra": map[string]any{},
			},
		},
	})
	if res.Code != 0 || lastCreate.HostConfig.NetworkMode != "host" || len(connects) != 0 {
		t.Fatalf("padded host mode mismatch: res=%+v create=%#v connects=%#v", res, lastCreate, connects)
	}
}

func TestHandleUpdateContainerEmptyFallbackImageReturnsImageRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	srvHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Id":     "cid-empty-image",
				"Name":   "/empty-img",
				"Config": map[string]any{"Image": "   "},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srvHTTP.Close()

	cli, err := dockercli.New(context.Background(), "tcp://"+srvHTTP.Listener.Addr().String())
	if err != nil {
		t.Fatalf("create dockercli: %v", err)
	}
	defer cli.Close()

	srv := &Server{
		docker: cli,
		logger: zap.NewNop(),
	}
	r := gin.New()
	r.POST("/containers/:id/update", srv.handleUpdateContainer())

	updBody, _ := json.Marshal(map[string]any{"image": "   "})
	req := httptest.NewRequest(http.MethodPost, "/containers/cid-empty-image/update", bytes.NewReader(updBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	var res BaseRes
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode response failed: %v, body=%s", err, rec.Body.String())
	}
	if res.Code != CodeImageRequired {
		t.Fatalf("expected CodeImageRequired (%d), got %+v", CodeImageRequired, res)
	}
}
