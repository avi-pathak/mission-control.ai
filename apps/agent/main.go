// Command mission-control-agent runs the host agent daemon.
//
// Usage:
//
//	mission-control-agent [--config agent.yaml]        run the daemon
//	mission-control-agent publish <path> [--session id] upload a file
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/avi-pathak/mission-control.ai/internal/agent"
	"github.com/avi-pathak/mission-control.ai/internal/config"
	"github.com/avi-pathak/mission-control.ai/internal/logging"
	"github.com/avi-pathak/mission-control.ai/internal/provider"
	"github.com/avi-pathak/mission-control.ai/internal/provider/claude"
	"github.com/avi-pathak/mission-control.ai/internal/provider/codex"
	"go.uber.org/zap"
)

func main() {
	// Subcommand dispatch: `publish` uploads a file, otherwise run the daemon.
	if len(os.Args) > 1 && os.Args[1] == "publish" {
		runPublish(os.Args[2:])
		return
	}
	runDaemon()
}

func runPublish(args []string) {
	fs := flag.NewFlagSet("publish", flag.ExitOnError)
	cfgPath := fs.String("config", "", "path to agent.yaml")
	session := fs.String("session", "", "session id to attach the file to")
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: mission-control-agent publish <path> [--session id] [--config agent.yaml]")
		os.Exit(2)
	}
	cfg, err := config.LoadAgent(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load config:", err)
		os.Exit(1)
	}
	path := fs.Arg(0)
	if err := agent.Publish(cfg.ServerURL, cfg.APIKey, path, *session, cfg.InsecureTLS); err != nil {
		fmt.Fprintln(os.Stderr, "publish:", err)
		os.Exit(1)
	}
	fmt.Printf("published %s\n", path)
}

func runDaemon() {
	cfgPath := flag.String("config", "", "path to agent.yaml")
	flag.Parse()

	cfg, err := config.LoadAgent(*cfgPath)
	if err != nil {
		panic(err)
	}

	log := logging.New(cfg.LogLevel)
	defer func() { _ = log.Sync() }()

	claude.Register()
	codex.Register()

	hostname := cfg.HostnameOverride
	if hostname == "" {
		hostname, _ = os.Hostname()
	}

	if err := agent.Bootstrap(&cfg, hostname, log); err != nil {
		log.Fatal("enrollment failed", zap.Error(err))
	}

	providers, err := provider.Build(cfg.Providers)
	if err != nil {
		log.Fatal("build providers", zap.Error(err))
	}

	rt := agent.New(cfg, log, providers)
	// Inject the resolved machine id into providers that need it.
	for _, p := range providers {
		if ma, ok := p.(provider.MachineAware); ok {
			ma.SetMachineID(rt.AgentID())
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("mission-control agent starting", zap.String("id", rt.AgentID()), zap.String("server", cfg.ServerURL))
	rt.Run(ctx)
}
