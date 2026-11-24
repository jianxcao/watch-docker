package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/jianxcao/watch-docker/backend/internal/api"
	"github.com/jianxcao/watch-docker/backend/internal/conf"
	"github.com/jianxcao/watch-docker/backend/internal/config"
	"github.com/jianxcao/watch-docker/backend/internal/dockercli"
	logger "github.com/jianxcao/watch-docker/backend/internal/logging"
	"github.com/jianxcao/watch-docker/backend/internal/notificationmanager"
	"github.com/jianxcao/watch-docker/backend/internal/notify"
	"github.com/jianxcao/watch-docker/backend/internal/registry"
	"github.com/jianxcao/watch-docker/backend/internal/scanner"
	"github.com/jianxcao/watch-docker/backend/internal/scheduler"
	"github.com/jianxcao/watch-docker/backend/internal/updater"

	"go.uber.org/zap"
)

func main() {

	configPath := path.Join(conf.EnvCfg.CONFIG_PATH, conf.EnvCfg.CONFIG_FILE)
	// init logger first
	log, err := logger.NewLogger("info", "")
	if err != nil {
		panic(fmt.Errorf("init logger: %w", err))
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		panic(fmt.Errorf("load config: %w", err))
	}
	// set log level
	if err := logger.SetLogLevel(cfg.Logging.Level); err != nil {
		panic(fmt.Errorf("set log level: %w", err))
	}
	defer log.Sync() //nolint:errcheck
	log.Info("starting watch-docker", zap.String("configPath", configPath))
	log.Info("version info", zap.String("version", conf.GetVersion()))
	// init long-lived clients
	dockerClient, err := dockercli.New(context.Background(), cfg.Docker.Host)
	if err != nil {
		log.Fatal("docker client init failed", logger.ZapErr(err))
	}
	reg := registry.New()
	sc := scanner.New(dockerClient, reg)

	// init notification system
	notifier := notify.New(config.Loader())
	notificationManager := notificationmanager.New(notifier, path.Join(conf.EnvCfg.CONFIG_PATH, "notification-history.json"))

	// share a single updater instance between scheduler and API router so per-container locks match
	upd := updater.New(dockerClient)

	// start scheduler
	sch := scheduler.New(log, sc, upd, notificationManager)
	sch.Start()

	r, closeStreams := api.NewRouterWithLifecycle(log, dockerClient, reg, sc, sch, upd)

	srv := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		logger.Logger.Info("http server starting", logger.ZapField("addr", cfg.Server.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Logger.Fatal("http server exited", logger.ZapErr(err))
		}
	}()

	// graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Logger.Info("shutting down http server")
	// stop scheduler and wait for running cron jobs
	sch.StopCron()
	// close notification manager (flush pending notifications)
	notificationManager.Close()
	// actively close long-lived SSE and WebSocket streams so srv.Shutdown completes immediately
	if closeStreams != nil {
		closeStreams()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Logger.Error("server shutdown error", logger.ZapErr(err))
	}
	if err := dockerClient.Close(); err != nil {
		logger.Logger.Warn("close docker client error", logger.ZapErr(err))
	}
}
