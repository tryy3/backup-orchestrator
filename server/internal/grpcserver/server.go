package grpcserver

import (
	"context"
	"log"
	"runtime/debug"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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

// NewGRPCServer creates and configures a grpc.Server with the BackupService registered.
func NewGRPCServer(db *database.DB, mgr *agentmgr.Manager, resolver *configpush.Resolver, hub *events.Hub) *grpc.Server {
	srv := &GRPCServer{
		db:       db,
		mgr:      mgr,
		resolver: resolver,
		hub:      hub,
	}

	grpcSrv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			srv.unaryAuthInterceptor(),
			unaryRecoveryInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			srv.streamAuthInterceptor(),
			streamRecoveryInterceptor(),
		),
	)
	backupv1.RegisterBackupServiceServer(grpcSrv, srv)
	return grpcSrv
}

// unaryRecoveryInterceptor returns an interceptor that recovers from panics in unary RPCs.
func unaryRecoveryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("gRPC unary panic recovered in %s: %v\n%s", info.FullMethod, r, debug.Stack())
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(ctx, req)
	}
}

// streamRecoveryInterceptor returns an interceptor that recovers from panics in streaming RPCs.
func streamRecoveryInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("gRPC stream panic recovered in %s: %v\n%s", info.FullMethod, r, debug.Stack())
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(srv, ss)
	}
}
