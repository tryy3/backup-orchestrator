package grpcserver

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	backupv1 "github.com/tryy3/backup-orchestrator/server/internal/gen/backup/v1"
)

// contextKey is a private type for context keys in this package.
type contextKey string

const agentIDKey contextKey = "agent_id"

// agentIDFromContext extracts the authenticated agent ID from the context.
func agentIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(agentIDKey).(string); ok {
		return v
	}
	return ""
}

// unaryAuthInterceptor returns a gRPC unary interceptor that validates API keys.
// The Register RPC is exempted from authentication.
func (s *GRPCServer) unaryAuthInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Skip auth for Register RPC.
		if info.FullMethod == backupv1.BackupService_Register_FullMethodName {
			return handler(ctx, req)
		}

		// Try to get API key from message fields first.
		var apiKey string
		switch r := req.(type) {
		case *backupv1.JobReport:
			apiKey = r.ApiKey
		case *backupv1.SnapshotReport:
			apiKey = r.ApiKey
		}

		// Fall back to metadata.
		if apiKey == "" {
			md, ok := metadata.FromIncomingContext(ctx)
			if ok {
				keys := md.Get("api_key")
				if len(keys) > 0 {
					apiKey = keys[0]
				}
			}
		}

		if apiKey == "" {
			return nil, status.Error(codes.Unauthenticated, "missing api_key")
		}

		agent, err := s.db.GetAgentByAPIKey(apiKey)
		if err != nil {
			return nil, status.Error(codes.Internal, "failed to validate api_key")
		}
		if agent == nil || agent.Status != "approved" {
			return nil, status.Error(codes.Unauthenticated, "invalid api_key")
		}

		ctx = context.WithValue(ctx, agentIDKey, agent.ID)
		return handler(ctx, req)
	}
}

// streamAuthInterceptor returns a gRPC stream interceptor that validates API keys.
// The Connect RPC handles its own auth within the stream handler.
func (s *GRPCServer) streamAuthInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// The Connect stream does its own authentication per-message,
		// because pending agents send messages without an API key.
		if info.FullMethod == backupv1.BackupService_Connect_FullMethodName {
			return handler(srv, ss)
		}

		// For any other streams, require auth via metadata.
		md, ok := metadata.FromIncomingContext(ss.Context())
		if !ok {
			return status.Error(codes.Unauthenticated, "missing metadata")
		}

		keys := md.Get("api_key")
		if len(keys) == 0 {
			return status.Error(codes.Unauthenticated, "missing api_key")
		}

		agent, err := s.db.GetAgentByAPIKey(keys[0])
		if err != nil {
			return status.Error(codes.Internal, "failed to validate api_key")
		}
		if agent == nil || agent.Status != "approved" {
			return status.Error(codes.Unauthenticated, "invalid api_key")
		}

		return handler(srv, ss)
	}
}
