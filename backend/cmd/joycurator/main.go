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

	base := "local"
	cwd, _ := os.Getwd()
	if filepath.Base(cwd) == "backend" {
		base = "../local"
	}
	if executable, err := os.Executable(); err == nil && filepath.Base(filepath.Dir(executable)) == "bin" && filepath.Base(filepath.Dir(filepath.Dir(executable))) == "local" {
		base = filepath.Dir(filepath.Dir(executable))
	} else if webui.Embedded && err == nil {
		base = filepath.Join(filepath.Dir(executable), "local")
	}
	base, _ = filepath.Abs(base)
	for _, d := range []string{"data", "media/inbox", "media/movies", "media/tv"} {
		if err := os.MkdirAll(filepath.Join(base, d), 0755); err != nil {
			slog.Error("创建目录失败", "error", err)
			os.Exit(1)
		}
	}
	roots := filepath.SplitList(env("JOYCURATOR_MEDIA_ROOTS", filepath.Join(base, "media")))
	a, err := app.Open(env("JOYCURATOR_DATA_DIR", filepath.Join(base, "data")), roots, strings.EqualFold(os.Getenv("JOYCURATOR_SECURE_COOKIE"), "true"))
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
