package main

import (
	"context"
	"log"
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
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Open database.
	db, err := database.New(cfg.DBPath, cfg.EncryptionKey)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	log.Printf("Database opened at %s", cfg.DBPath)

	grpcLis, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", ":"+cfg.GRPCPort)
	if err != nil {
		_ = db.Close()
		log.Fatalf("Failed to listen on gRPC port %s: %v", cfg.GRPCPort, err)
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
	router := api.NewRouter(db, mgr, resolver, hub)
	httpSrv := &http.Server{
		Addr:         ":" + cfg.HTTPPort,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start gRPC server.
	go func() {
		log.Printf("gRPC server listening on :%s", cfg.GRPCPort)
		if err := grpcSrv.Serve(grpcLis); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()

	// Start HTTP server.
	go func() {
		log.Printf("HTTP server listening on :%s", cfg.HTTPPort)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// Wait for shutdown signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Received signal %v, shutting down...", sig)

	// Graceful shutdown.
	grpcDone := make(chan struct{})
	go func() {
		grpcSrv.GracefulStop()
		close(grpcDone)
	}()
	select {
	case <-grpcDone:
	case <-time.After(15 * time.Second):
		log.Println("gRPC graceful stop timed out, forcing stop")
		grpcSrv.Stop()
	}
	log.Println("gRPC server stopped")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}
	log.Println("HTTP server stopped")

	// Shut down agent manager (closes all agent send channels).
	mgr.Close()
	log.Println("Agent manager stopped")

	// Shut down event hub (closes all WebSocket client channels).
	hub.Close()
	log.Println("Event hub stopped")

	log.Println("Server shutdown complete")
}
