//go:build desktop

package main

import (
	"log/slog"
	"os"

	"github.com/SDK-E/dnser/internal/config"
	"github.com/SDK-E/dnser/internal/desktop"
)

var version = "dev"

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	path, err := config.DefaultPath()
	if err != nil {
		slog.Error("resolve config path", "err", err)
		os.Exit(1)
	}
	store, err := config.Open(path)
	if err != nil {
		slog.Error("load config", "path", path, "err", err)
		os.Exit(1)
	}

	os.Exit(desktop.Run(desktop.Options{
		Store:   store,
		Version: version,
	}))
}
