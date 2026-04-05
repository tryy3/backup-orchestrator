package grpcserver

import (
	"google.golang.org/grpc"

	backupv1 "github.com/tryy3/backup-orchestrator/server/internal/gen/backup/v1"

	"github.com/tryy3/backup-orchestrator/server/internal/agentmgr"
	"github.com/tryy3/backup-orchestrator/server/internal/configpush"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
)

// GRPCServer implements the BackupServiceServer gRPC interface.
type GRPCServer struct {
	backupv1.UnimplementedBackupServiceServer
	db       *database.DB
	mgr      *agentmgr.Manager
	resolver *configpush.Resolver
}

// New creates a new GRPCServer.
func New(db *database.DB, mgr *agentmgr.Manager, resolver *configpush.Resolver) *GRPCServer {
	return &GRPCServer{
		db:       db,
		mgr:      mgr,
		resolver: resolver,
	}
}

// NewGRPCServer creates and configures a grpc.Server with the BackupService registered.
func NewGRPCServer(db *database.DB, mgr *agentmgr.Manager, resolver *configpush.Resolver) *grpc.Server {
	srv := &GRPCServer{
		db:       db,
		mgr:      mgr,
		resolver: resolver,
	}

	grpcSrv := grpc.NewServer(
		grpc.UnaryInterceptor(srv.unaryAuthInterceptor()),
		grpc.StreamInterceptor(srv.streamAuthInterceptor()),
	)
	backupv1.RegisterBackupServiceServer(grpcSrv, srv)
	return grpcSrv
}
