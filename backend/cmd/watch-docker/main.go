package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
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

func parseCLIArgs(args []string) (localTest bool, err error) {
	fs := flag.NewFlagSet("watch-docker", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.BoolVar(&localTest, "local-test", false, "启用本地测试启动前安全配置校验")
	if err := fs.Parse(args); err != nil {
		return false, err
	}
	return localTest, nil
}

func isEnvExplicitTrue(key string) bool {
	val, ok := os.LookupEnv(key)
	if !ok {
		return false
	}
	b, err := strconv.ParseBool(strings.TrimSpace(val))
	return err == nil && b
}

func validateLocalTestConfig(cfg *conf.EnvConfig, shellRequested bool) error {
	if cfg == nil {
		return fmt.Errorf("本地测试启动失败：配置未初始化")
	}
	if strings.TrimSpace(cfg.USER_NAME) == "" {
		return fmt.Errorf("本地测试启动失败：有效用户名不能为空，请设置 USER_NAME 或配置 app.yaml 中的 username")
	}
	trimmedPass := strings.TrimSpace(cfg.USER_PASSWORD)
	if trimmedPass == "" || trimmedPass == "admin" {
		return fmt.Errorf("本地测试启动失败：未配置有效登录密码或仍在使用默认密码，请设置非默认的 USER_PASSWORD 环境变量或在 %s/app.yaml 中配置 password", cfg.CONFIG_PATH)
	}
	if cfg.IS_OPEN_DOCKER_SHELL && !shellRequested {
		return fmt.Errorf("本地测试启动失败：当前有效配置启用了容器 Shell，如需在本地测试中启用请显式设置 IS_OPEN_DOCKER_SHELL=true 或在 app.yaml 中关闭 enable_docker_shell")
	}
	return nil
}

func main() {
	localTest, err := parseCLIArgs(os.Args[1:])
	if err != nil {
		os.Exit(2)
	}
	if localTest {
		if err := validateLocalTestConfig(conf.EnvCfg, isEnvExplicitTrue("IS_OPEN_DOCKER_SHELL")); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	}

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
	// 1. 先主动关闭长连接 SSE 与 WebSocket 流，随后关闭 HTTP 服务，阻止新请求触发调度/通知
	if closeStreams != nil {
		closeStreams()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Logger.Error("server shutdown error", logger.ZapErr(err))
	}
	// 2. HTTP 不再接新请求后停止调度器并等待进行中的扫描更新退出
	sch.StopCron()
	// 3. 调度器彻底停止后再关闭通知管理器并刷新待发送通知
	notificationManager.Close()
	if err := dockerClient.Close(); err != nil {
		logger.Logger.Warn("close docker client error", logger.ZapErr(err))
	}
}
