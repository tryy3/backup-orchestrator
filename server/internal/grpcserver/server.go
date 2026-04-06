package grpcserver

import (
	"google.golang.org/grpc"

	backupv1 "github.com/tryy3/backup-orchestrator/server/internal/gen/backup/v1"

	"github.com/tryy3/backup-orchestrator/server/internal/agentmgr"
	"github.com/tryy3/backup-orchestrator/server/internal/configpush"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
	"github.com/tryy3/backup-orchestrator/server/internal/events"
)

// GRPCServer implements the BackupServiceServer gRPC interface.
type GRPCServer struct {
	backupv1.UnimplementedBackupServiceServer
	db       *database.DB
	mgr      *agentmgr.Manager
	resolver *configpush.Resolver
	hub      *events.Hub
}

// New creates a new GRPCServer.
func New(db *database.DB, mgr *agentmgr.Manager, resolver *configpush.Resolver, hub *events.Hub) *GRPCServer {
	return &GRPCServer{
		db:       db,
		mgr:      mgr,
		resolver: resolver,
		hub:      hub,
	}
}

// NewGRPCServer creates and configures a grpc.Server with the BackupService registered.
func NewGRPCServer(db *database.DB, mgr *agentmgr.Manager, resolver *configpush.Resolver, hub *events.Hub) *grpc.Server {
	srv := &GRPCServer{
		db:       db,
		mgr:      mgr,
		resolver: resolver,
		hub:      hub,
	}

	grpcSrv := grpc.NewServer(
		grpc.UnaryInterceptor(srv.unaryAuthInterceptor()),
		grpc.StreamInterceptor(srv.streamAuthInterceptor()),
	)
	backupv1.RegisterBackupServiceServer(grpcSrv, srv)
	return grpcSrv
}
