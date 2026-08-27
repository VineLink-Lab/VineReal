// Command vinereal-server runs the REALITY-fronted reverse proxy.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/VineLink-Lab/VineReal/server/config"
	"github.com/VineLink-Lab/VineReal/server/proxy"
)

// version is stamped at build time via
// -ldflags "-X main.version=$(git describe --tags)" and defaults to "dev" for
// local, untagged builds.
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	configPath := flag.String("config", "config.yaml", "path to the server YAML config")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "path", *configPath, "err", err)
		os.Exit(1)
	}

	ln, err := proxy.NewListener(cfg)
	if err != nil {
		slog.Error("failed to start listener", "listen", cfg.Listen, "err", err)
		os.Exit(1)
	}
	slog.Info("vinereal-server listening", "listen", cfg.Listen, "upstream", cfg.Upstream, "dest", cfg.Dest)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() { serveErr <- proxy.Serve(ln, cfg) }()

	select {
	case <-ctx.Done():
		slog.Info("shutting down")
		ln.Close()
		<-serveErr
	case err := <-serveErr:
		if err != nil {
			slog.Error("listener stopped", "err", err)
			os.Exit(1)
		}
	}
}
