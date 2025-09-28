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

	cfg, err := config.Load(configPath)
	if err != nil {
		panic(fmt.Errorf("load config: %w", err))
	}
	log, err := logger.NewLogger(cfg.Logging.Level, "")
	if err != nil {
		panic(fmt.Errorf("init logger: %w", err))
	}
	defer log.Sync() //nolint:errcheck
	log.Info("starting watch-docker", zap.String("configPath", configPath))
	// init long-lived clients
	dockerClient, err := dockercli.New(context.Background(), cfg.Docker.Host)
	if err != nil {
		log.Fatal("docker client init failed", logger.ZapErr(err))
	}
	reg := registry.New()
	sc := scanner.New(dockerClient, reg)

	// init notification system
	notifier := notify.New(func() *config.Config { return cfg })
	notificationManager := notificationmanager.New(notifier, path.Join(conf.EnvCfg.CONFIG_PATH, "notification-history.json"))

	// start scheduler
	sch := scheduler.New(log, sc, updater.New(dockerClient), notificationManager)
	sch.Start()

	r := api.NewRouter(log, dockerClient, reg, sc, sch)

	srv := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	logger.Logger.Info("shutting down http server")
	// stop scheduler
	sch.Stop()
	// close notification manager (flush pending notifications)
	notificationManager.Close()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Logger.Error("server shutdown error", logger.ZapErr(err))
	}
}
