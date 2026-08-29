package grpcserver

import "github.com/tryy3/backup-orchestrator/server/internal/configpush"

// requestConfigPushOnConnect schedules a config push after an agent connects.
// Must be called only after agentmgr.Register so IsOnline is true.
func requestConfigPushOnConnect(resolver *configpush.Resolver, agentID, status string) {
	if status != "approved" {
		return
	}
	resolver.RequestPush(agentID)
}
