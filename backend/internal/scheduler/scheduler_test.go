package scheduler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	"github.com/jianxcao/watch-docker/backend/internal/scanner"
	"go.uber.org/zap"
)

func TestStopCronCancelsAndWaitsForHTTPRunScanAndUpdate(t *testing.T) {
	inScan := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/containers/json") {
			close(inScan)
			<-r.Context().Done()
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	dockerCli, err := dockercli.New(context.Background(), "tcp://"+srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer dockerCli.Close()

	sc := scanner.New(dockerCli, nil)
	sch := New(zap.NewNop(), sc, nil, nil)

	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		sch.RunScanAndUpdate(context.Background())
	}()

	select {
	case <-inScan:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for RunScanAndUpdate to enter ScanOnce")
	}

	stopDone := make(chan struct{})
	go func() {
		defer close(stopDone)
		sch.StopCron()
	}()

	select {
	case <-stopDone:
	case <-time.After(3 * time.Second):
		t.Fatal("StopCron timed out waiting for RunScanAndUpdate cancellation")
	}

	select {
	case <-runDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("StopCron returned before in-flight RunScanAndUpdate exited")
	}
}

func TestRunScanAndUpdateAfterStopCronIsNoOp(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/containers/json") {
			hits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("[]"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	dockerCli, err := dockercli.New(context.Background(), "tcp://"+srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer dockerCli.Close()

	sc := scanner.New(dockerCli, nil)
	sch := New(zap.NewNop(), sc, nil, nil)
	sch.StopCron()
	sch.RunScanAndUpdate(context.Background())

	if hits.Load() != 0 {
		t.Fatalf("expected 0 /containers/json calls after StopCron, got %d", hits.Load())
	}
}
