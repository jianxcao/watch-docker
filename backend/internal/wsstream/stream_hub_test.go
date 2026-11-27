package wsstream

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type finiteReader struct {
	sent bool
}

func (r *finiteReader) Read(ctx context.Context) (string, error) {
	if !r.sent {
		r.sent = true
		return "final-chunk", nil
	}
	return "", io.EOF
}

func (r *finiteReader) Close() error { return nil }

type finiteSource struct {
	key string
}

func (s *finiteSource) Start(ctx context.Context) (StreamReader[string], error) {
	return &finiteReader{}, nil
}

func (s *finiteSource) Stop() error { return nil }

func (s *finiteSource) GetKey() string { return s.key }

func TestFiniteStreamHubClosesPromptlyWithoutZombieState(t *testing.T) {
	mgr := NewStreamManager[string]()

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	doneHubCh := make(chan *StreamHub[string], 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade error: %v", err)
			return
		}
		hub := mgr.GetOrCreateHub("finite-test", func() StreamSource[string] {
			return &finiteSource{key: "finite-test"}
		})
		client := NewClient(conn, hub, "client-1")
		if !hub.RegisterClient(client) {
			t.Errorf("expected RegisterClient to succeed")
			return
		}
		go client.WritePump()
		go client.ReadPump()
		doneHubCh <- hub
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws failed: %v", err)
	}
	defer conn.Close()

	// 即使客户端未主动断开，也应先收到终态消息，并在 100ms 排水期后自动关闭 Hub（不留僵尸 Hub）
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("expected final message before close, got %v", err)
	}
	if string(msg) != "final-chunk" {
		t.Fatalf("unexpected message: %q", string(msg))
	}

	hub := <-doneHubCh
	select {
	case <-hub.done:
	case <-time.After(1 * time.Second):
		t.Fatal("expected finite StreamHub to terminate after drain window")
	}

	if !hub.IsClosed() {
		t.Fatal("expected terminated hub to report IsClosed=true")
	}
	if mgr.GetHubCount() != 0 {
		t.Fatalf("expected StreamManager hub count 0, got %d", mgr.GetHubCount())
	}
}

type blockingReader struct{}

func (r *blockingReader) Read(ctx context.Context) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func (r *blockingReader) Close() error { return nil }

type blockingSource struct {
	key string
}

func (s *blockingSource) Start(ctx context.Context) (StreamReader[string], error) {
	return &blockingReader{}, nil
}

func (s *blockingSource) Stop() error { return nil }

func (s *blockingSource) GetKey() string { return s.key }

func TestStreamHubBroadcastConcurrentUnregisterAndClose(t *testing.T) {
	mgr := NewStreamManager[string]()
	hub := mgr.GetOrCreateHub("stress-test", func() StreamSource[string] {
		return &blockingSource{key: "stress-test"}
	})

	anchor := &Client[string]{
		send: make(chan string, 64),
		hub:  hub,
		id:   "anchor",
	}
	if !hub.RegisterClient(anchor) {
		t.Fatal("expected anchor RegisterClient to succeed")
	}
	var anchorWG sync.WaitGroup
	anchorWG.Add(1)
	go func() {
		defer anchorWG.Done()
		for range anchor.send {
		}
	}()

	var workerWG sync.WaitGroup
	stopWorkers := make(chan struct{})

	// 并发注册和注销临时客户端，同时触发 unregister 关闭 client.send
	for w := 0; w < 6; w++ {
		workerWG.Add(1)
		go func(workerID int) {
			defer workerWG.Done()
			i := 0
			for {
				select {
				case <-stopWorkers:
					return
				default:
				}
				client := &Client[string]{
					send: make(chan string, 8),
					hub:  hub,
					id:   fmt.Sprintf("worker-%d-%d", workerID, i),
				}
				i++
				if !hub.RegisterClient(client) {
					return
				}
				hub.UnregisterClient(client)
			}
		}(w)
	}

	// 并发广播消息，覆盖与 unregister 和 sourceDone 并发关闭通道的场景
	var broadcastWG sync.WaitGroup
	for b := 0; b < 4; b++ {
		broadcastWG.Add(1)
		go func() {
			defer broadcastWG.Done()
			for i := 0; i < 400; i++ {
				hub.broadcast("msg")
			}
		}()
	}

	time.Sleep(15 * time.Millisecond)
	close(stopWorkers)
	workerWG.Wait()

	// 在广播仍在继续时触发 sourceDone 关闭剩余客户端
	hub.closeAllClients()
	broadcastWG.Wait()

	select {
	case <-hub.done:
	case <-time.After(2 * time.Second):
		t.Fatal("expected StreamHub to exit after closeAllClients")
	}
	anchorWG.Wait()

	if !hub.IsClosed() {
		t.Fatal("expected hub to be closed")
	}
	if mgr.GetHubCount() != 0 {
		t.Fatalf("expected StreamManager hub count 0, got %d", mgr.GetHubCount())
	}
}

type stopBlockingSource struct {
	key           string
	startedCh     chan struct{}
	inStopCh      chan struct{}
	releaseStopCh chan struct{}
	stopOnce      sync.Once
}

func (s *stopBlockingSource) Start(ctx context.Context) (StreamReader[string], error) {
	select {
	case <-s.startedCh:
	default:
		close(s.startedCh)
	}
	return &blockingReader{}, nil
}

func (s *stopBlockingSource) Stop() error {
	s.stopOnce.Do(func() {
		close(s.inStopCh)
	})
	<-s.releaseStopCh
	return nil
}

func (s *stopBlockingSource) GetKey() string { return s.key }

type trackedRejectSource struct {
	key         string
	startCalled *atomic.Int32
}

func (s *trackedRejectSource) Start(ctx context.Context) (StreamReader[string], error) {
	s.startCalled.Add(1)
	return &blockingReader{}, nil
}

func (s *trackedRejectSource) Stop() error  { return nil }
func (s *trackedRejectSource) GetKey() string { return s.key }

func TestStreamManagerClosePreventsNewStreamsWhileStopping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mgr := NewStreamManager[string]()

	oldSrc := &stopBlockingSource{
		key:           "key-1",
		startedCh:     make(chan struct{}),
		inStopCh:      make(chan struct{}),
		releaseStopCh: make(chan struct{}),
	}

	wsUpgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	// 建立 key-1 的活跃 WebSocket 客户端
	oldSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade key-1 failed: %v", err)
			return
		}
		mgr.StartHub(conn, "key-1", func() StreamSource[string] { return oldSrc })
	}))
	defer oldSrv.Close()

	oldConn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(oldSrv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial key-1 failed: %v", err)
	}
	defer oldConn.Close()

	select {
	case <-oldSrc.startedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for key-1 source Start")
	}

	// 启动并发 Close()，并阻塞在 oldSrc.Stop()
	closeDone := make(chan struct{})
	go func() {
		defer close(closeDone)
		mgr.Close()
	}()

	select {
	case <-oldSrc.inStopCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for oldSrc.Stop() to begin")
	}

	// 场景 A：已在停机前完成协议升级的连接进入 StartHub(key-2)，不得创建新 Hub/启动新数据源，且连接必须关闭
	var factoryCallsA atomic.Int32
	var startCallsA atomic.Int32
	startHubEntered := make(chan struct{})
	preUpgradedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, uerr := wsUpgrader.Upgrade(w, r, nil)
		if uerr != nil {
			t.Errorf("pre-upgrade key-2 failed: %v", uerr)
			return
		}
		mgr.StartHub(conn, "key-2-starthub", func() StreamSource[string] {
			factoryCallsA.Add(1)
			return &trackedRejectSource{key: "key-2-starthub", startCalled: &startCallsA}
		})
		close(startHubEntered)
	}))
	defer preUpgradedSrv.Close()

	connA, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(preUpgradedSrv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial preUpgradedSrv failed: %v", err)
	}
	defer connA.Close()

	select {
	case <-startHubEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for StartHub")
	}
	_ = connA.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, rerr := connA.ReadMessage(); rerr == nil {
		t.Fatal("expected pre-upgraded connection to be closed during StreamManager shutdown")
	}
	if factoryCallsA.Load() != 0 || startCallsA.Load() != 0 {
		t.Fatalf("expected no new source created/started during Close, factory=%d start=%d", factoryCallsA.Load(), startCallsA.Load())
	}

	// 场景 B：通过 HandleWebSocket 发起的新握手在停机期间应被拒绝，且同样不启动新数据源
	var startCallsB atomic.Int32
	router := gin.New()
	router.GET("/ws", func(c *gin.Context) {
		mgr.HandleWebSocket(c, "key-2-handle", func() StreamSource[string] {
			return &trackedRejectSource{key: "key-2-handle", startCalled: &startCallsB}
		})
	})
	handleSrv := httptest.NewServer(router)
	defer handleSrv.Close()

	connB, respB, errB := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(handleSrv.URL, "http")+"/ws", nil)
	if errB == nil {
		_ = connB.Close()
		t.Fatal("expected HandleWebSocket dial to fail during shutdown")
	}
	if respB == nil || respB.StatusCode != http.StatusServiceUnavailable {
		status := 0
		if respB != nil {
			status = respB.StatusCode
		}
		t.Fatalf("expected 503 Service Unavailable during shutdown, got err=%v status=%d", errB, status)
	}
	if startCallsB.Load() != 0 {
		t.Fatalf("expected 0 source Start calls during shutdown, got %d", startCallsB.Load())
	}

	// 放行旧数据源 Stop() 后 Close() 正常返回且 Hub 数为 0
	close(oldSrc.releaseStopCh)
	select {
	case <-closeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for mgr.Close() to finish")
	}
	if mgr.GetHubCount() != 0 {
		t.Fatalf("expected 0 hubs after Close completes, got %d", mgr.GetHubCount())
	}
}
