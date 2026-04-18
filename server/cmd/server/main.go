package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tryy3/backup-orchestrator/server/internal/agentmgr"
	"github.com/tryy3/backup-orchestrator/server/internal/api"
	"github.com/tryy3/backup-orchestrator/server/internal/config"
	"github.com/tryy3/backup-orchestrator/server/internal/configpush"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
	"github.com/tryy3/backup-orchestrator/server/internal/events"
	"github.com/tryy3/backup-orchestrator/server/internal/grpcserver"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Open database.
	db, err := database.New(cfg.DBPath, cfg.EncryptionKey)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	slog.Info("database opened", "path", cfg.DBPath)

	grpcLis, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", ":"+cfg.GRPCPort)
	if err != nil {
		_ = db.Close()
		slog.Error("failed to listen on gRPC port", "port", cfg.GRPCPort, "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	// Create agent manager.
	mgr := agentmgr.New()

	// Create config resolver.
	resolver := configpush.New(db, mgr)

	// Create event hub for WebSocket push.
	hub := events.NewHub()

	// Create gRPC server.
	grpcSrv := grpcserver.NewGRPCServer(db, mgr, resolver, hub)

	// Create HTTP server.
	router := api.NewRouter(db, mgr, resolver, hub, cfg.AllowedOrigins)
	httpSrv := &http.Server{
		Addr:         ":" + cfg.HTTPPort,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start gRPC server.
	go func() {
		slog.Info("gRPC server listening", "port", cfg.GRPCPort)
		if err := grpcSrv.Serve(grpcLis); err != nil {
			slog.Error("gRPC server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Start HTTP server.
	go func() {
		slog.Info("HTTP server listening", "port", cfg.HTTPPort)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	slog.Info("received signal, shutting down", "signal", sig)

	// Graceful shutdown.
	grpcDone := make(chan struct{})
	go func() {
		grpcSrv.GracefulStop()
		close(grpcDone)
	}()
	select {
	case <-grpcDone:
	case <-time.After(15 * time.Second):
		slog.Warn("gRPC graceful stop timed out, forcing stop")
		grpcSrv.Stop()
	}
	slog.Info("gRPC server stopped")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}
	slog.Info("HTTP server stopped")

	// Shut down agent manager (closes all agent send channels).
	mgr.Close()
	slog.Info("agent manager stopped")

	// Shut down event hub (closes all WebSocket client channels).
	hub.Close()
	slog.Info("event hub stopped")

	slog.Info("server shutdown complete")
}
