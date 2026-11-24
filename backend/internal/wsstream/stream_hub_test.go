package wsstream

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
