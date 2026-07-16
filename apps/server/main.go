// Command mission-control-server runs the Mission Control control plane.
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/avi-pathak/mission-control.ai/internal/config"
	"github.com/avi-pathak/mission-control.ai/internal/logging"
	"github.com/avi-pathak/mission-control.ai/internal/server"
	"github.com/avi-pathak/mission-control.ai/internal/store"
	"go.uber.org/zap"
)

func main() {
	cfgPath := flag.String("config", "", "path to server.yaml")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		panic(err)
	}

	log := logging.New(cfg.LogLevel)
	defer func() { _ = log.Sync() }()

	st, err := store.Open(cfg.DatabaseURL, log)
	if err != nil {
		log.Fatal("open store", zap.Error(err))
	}

	srv := server.New(cfg, log, st)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.Run(ctx); err != nil {
		log.Fatal("server exited", zap.Error(err))
	}
	log.Info("shutdown complete")
}
