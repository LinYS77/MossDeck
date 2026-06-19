// Command server starts the homepage backend. It loads configuration,
// initializes structured logging, opens and migrates the SQLite database,
// and runs the HTTP server with graceful shutdown on SIGINT/SIGTERM.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/linyusheng/homepage/internal/config"
	"github.com/linyusheng/homepage/internal/db"
	"github.com/linyusheng/homepage/internal/logging"
	"github.com/linyusheng/homepage/internal/server"
)

func main() {
	if err := run(); err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger, err := logging.New(cfg.LogLevel, cfg.LogFormat)
	if err != nil {
		return err
	}
	// Make the configured logger the default so libraries that use slog
	// (including our migration runner) emit through the same sink.
	slog.SetDefault(logger)

	// Ensure the directory for the SQLite file exists.
	if dir := filepath.Dir(cfg.DatabasePath); dir != "" && dir != "." && dir != ":" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	// Stop on the first interrupt or terminate signal.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	database, err := db.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := database.Close(); cerr != nil {
			logger.Error("closing database", "error", cerr)
		}
	}()

	if err := db.Migrate(ctx, database); err != nil {
		return err
	}
	logger.Info("database ready", "path", cfg.DatabasePath)

	srv := server.New(ctx, cfg, database, logger)

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", cfg.HTTPAddr, "env", cfg.App)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serveErr:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("shutdown complete")
	return nil
}
