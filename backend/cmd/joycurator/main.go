package main

import (
	"context"
	"flag"
	"fmt"
	"joycurator/internal/app"
	"joycurator/internal/webui"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func env(name, fallback string) string {
	if s := os.Getenv(name); s != "" {
		return s
	}
	return fallback
}

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "显示版本")
	flag.Parse()
	if *showVersion {
		fmt.Println("JoyCurator", version)
		return
	}

	base := filepath.Join("..", "local")
	if webui.Embedded {
		executable, err := os.Executable()
		if err != nil {
			slog.Error("无法定位程序目录", "error", err)
			os.Exit(1)
		}
		base = filepath.Join(filepath.Dir(executable), "local")
	}
	base, err := filepath.Abs(base)
	if err != nil {
		slog.Error("无法解析数据目录", "error", err)
		os.Exit(1)
	}
	dataDir := env("JOYCURATOR_DATA_DIR", filepath.Join(base, "data"))
	sourceDir := env("JOYCURATOR_SOURCE_DIR", filepath.Join(base, "source"))
	targetDir := env("JOYCURATOR_TARGET_DIR", filepath.Join(base, "target"))
	for _, dir := range []string{sourceDir, targetDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			slog.Error("创建目录失败", "error", err)
			os.Exit(1)
		}
	}
	a, err := app.Open(dataDir, sourceDir, targetDir, strings.EqualFold(os.Getenv("JOYCURATOR_SECURE_COOKIE"), "true"))
	if err != nil {
		slog.Error("启动失败", "error", err)
		os.Exit(1)
	}
	srv := &http.Server{Addr: env("JOYCURATOR_ADDR", "127.0.0.1:8081"), Handler: a.Handler(webui.Files()), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	slog.Info("JoyCurator 已启动", "address", srv.Addr)
	if err = srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("服务失败", "error", err)
	}
	if err = a.Close(); err != nil {
		slog.Error("关闭失败", "error", err)
	}
}
