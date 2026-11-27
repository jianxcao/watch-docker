package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jianxcao/watch-docker/backend/internal/config"
)

func TestNotifierSendWithLegacyAtPrefixURLAndValidation(t *testing.T) {
	var gotTitle string
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotTitle = r.URL.Query().Get("title")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := New(func() *config.Config {
		return &config.Config{
			Notify: config.NotificationConfig{
				IsEnable: true,
				Method:   http.MethodGet,
				URL:      "@" + srv.URL + "/hook?title={title}",
			},
		}
	})

	if err := n.Send(context.Background(), "Watch Update", "body", "", ""); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if gotPath != "/hook" || gotTitle != "Watch Update" {
		t.Fatalf("got path=%q title=%q, want /hook and 'Watch Update'", gotPath, gotTitle)
	}

	// 反例：独立配置加载器传入裸 @ 或 @ftp:// 必须拒绝
	for _, badURL := range []string{"@", "@@https://notify.example/hook", "@ftp://notify.example/hook"} {
		badNotifier := New(func() *config.Config {
			return &config.Config{
				Notify: config.NotificationConfig{
					IsEnable: true,
					Method:   http.MethodGet,
					URL:      badURL,
				},
			}
		})
		if err := badNotifier.Send(context.Background(), "t", "c", "", ""); err == nil {
			t.Fatalf("expected Send to reject %q", badURL)
		}
	}
}
