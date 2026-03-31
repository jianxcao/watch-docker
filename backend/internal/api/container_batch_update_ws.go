package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jianxcao/watch-docker/backend/internal/config"
	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"github.com/jianxcao/watch-docker/backend/internal/updater"
	"go.uber.org/zap"
)

type batchUpdateMessage struct {
	Type          string `json:"type"`
	ContainerID   string `json:"containerId,omitempty"`
	ContainerName string `json:"containerName,omitempty"`
	Image         string `json:"image,omitempty"`
	Index         int    `json:"index,omitempty"`
	Total         int    `json:"total,omitempty"`
	Step          string `json:"step,omitempty"`
	Message       string `json:"message,omitempty"`
	Success       *bool  `json:"success,omitempty"`
	Error         string `json:"error,omitempty"`

	// pull progress fields
	LayerID string `json:"layerId,omitempty"`
	Status  string `json:"status,omitempty"`
	Current int64  `json:"current,omitempty"`
	TotalBytes int64  `json:"totalBytes,omitempty"`

	// scan result
	Containers []batchUpdateContainerInfo `json:"containers,omitempty"`

	// complete result
	Updated []string          `json:"updated,omitempty"`
	Failed  map[string]string `json:"failed,omitempty"`
}

type batchUpdateContainerInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Image string `json:"image"`
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	HandshakeTimeout:  10 * time.Second,
	ReadBufferSize:    4096,
	WriteBufferSize:   4096,
	EnableCompression: true,
}

func (s *Server) handleBatchUpdateWebSocket() gin.HandlerFunc {
	return func(c *gin.Context) {
		conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			logger.Logger.Error("WebSocket upgrade failed", zap.Error(err))
			return
		}
		defer conn.Close()

		var writeMu sync.Mutex
		sendMsg := func(msg batchUpdateMessage) {
			writeMu.Lock()
			defer writeMu.Unlock()
			data, err := json.Marshal(msg)
			if err != nil {
				logger.Logger.Error("marshal batch update message failed", zap.Error(err))
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				logger.Logger.Error("write batch update message failed", zap.Error(err))
			}
		}

		// keep-alive: drain pings / close from client
		ctx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()
		go func() {
			defer cancel()
			for {
				_, _, err := conn.ReadMessage()
				if err != nil {
					return
				}
			}
		}()

		cfg := config.Get()

		// 1. scan
		sendMsg(batchUpdateMessage{Type: "scan_start", Message: "正在扫描可更新容器..."})

		scanCtx, scanCancel := context.WithTimeout(ctx, 2*time.Minute)
		statuses, err := s.scanner.ScanOnce(scanCtx, true, cfg.Scan.Concurrency, true, true)
		scanCancel()
		if err != nil {
			sendMsg(batchUpdateMessage{Type: "error", Message: "扫描失败: " + err.Error()})
			return
		}

		type updateTarget struct {
			ID    string
			Name  string
			Image string
		}

		var targets []updateTarget
		for _, st := range statuses {
			if st.Skipped || st.Status != "UpdateAvailable" {
				continue
			}
			if !st.Running && !cfg.Docker.IncludeStopped {
				continue
			}
			targets = append(targets, updateTarget{ID: st.ID, Name: st.Name, Image: st.Image})
		}

		// build container info list
		containerInfos := make([]batchUpdateContainerInfo, len(targets))
		for i, t := range targets {
			containerInfos[i] = batchUpdateContainerInfo{ID: t.ID, Name: t.Name, Image: t.Image}
		}

		sendMsg(batchUpdateMessage{
			Type:       "scan_complete",
			Total:      len(targets),
			Containers: containerInfos,
		})

		if len(targets) == 0 {
			sendMsg(batchUpdateMessage{Type: "complete", Total: 0, Updated: []string{}, Failed: map[string]string{}})
			return
		}

		// 2. update each container
		updatedList := make([]string, 0)
		failedMap := make(map[string]string)

		for i, target := range targets {
			select {
			case <-ctx.Done():
				sendMsg(batchUpdateMessage{Type: "error", Message: "操作已取消"})
				return
			default:
			}

			sendMsg(batchUpdateMessage{
				Type:          "container_start",
				ContainerID:   target.ID,
				ContainerName: target.Name,
				Image:         target.Image,
				Index:         i + 1,
				Total:         len(targets),
			})

			updateCtx, updateCancel := context.WithTimeout(ctx, 5*time.Minute)
			updateErr := s.updater.UpdateContainerWithProgress(updateCtx, target.ID, target.Image, &updater.UpdateProgressCallback{
				OnStep: func(step, message string) {
					sendMsg(batchUpdateMessage{
						Type:          "step",
						ContainerID:   target.ID,
						ContainerName: target.Name,
						Step:          step,
						Message:       message,
					})
				},
				OnPullProgress: func(progress dockercli.PullProgress) {
					var current, total int64
					if progress.ProgressDetail != nil {
						current = progress.ProgressDetail.Current
						total = progress.ProgressDetail.Total
					}
					sendMsg(batchUpdateMessage{
						Type:        "pull_progress",
						ContainerID: target.ID,
						LayerID:     progress.ID,
						Status:      progress.Status,
						Current:     current,
						TotalBytes:  total,
					})
				},
			})
			updateCancel()

			success := updateErr == nil
			completeMsg := batchUpdateMessage{
				Type:          "container_complete",
				ContainerID:   target.ID,
				ContainerName: target.Name,
				Success:       &success,
			}

			if updateErr != nil {
				completeMsg.Error = updateErr.Error()
				failedMap[target.Name] = updateErr.Error()
				logger.Logger.Error("batch update container failed",
					zap.String("container", target.Name),
					zap.String("image", target.Image),
					zap.Error(updateErr))
			} else {
				updatedList = append(updatedList, target.Name)
			}
			sendMsg(completeMsg)
		}

		// 3. complete
		sendMsg(batchUpdateMessage{
			Type:    "complete",
			Total:   len(targets),
			Updated: updatedList,
			Failed:  failedMap,
		})
	}
}
