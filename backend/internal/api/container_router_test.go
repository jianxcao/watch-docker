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
