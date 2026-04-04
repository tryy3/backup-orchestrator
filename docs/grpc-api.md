# gRPC API Definition

Communication contract between agent and server.

## Service Overview

```
┌─────────────────────────────────────────────┐
│            BackupService (gRPC)              │
├─────────────────────────────────────────────┤
│ Enrollment:                                 │
│   Register(RegisterReq) -> RegisterResp     │
│                                             │
│ Persistent connection:                      │
│   Connect(stream AgentMsg) -> stream SrvMsg │
│                                             │
│ Job reporting:                              │
│   ReportJob(JobReport) -> ReportAck         │
│                                             │
│ Snapshot queries (agent -> server cache):   │
│   ReportSnapshots(SnapshotList) -> Ack      │
└─────────────────────────────────────────────┘
```

## Protobuf Definitions

```protobuf
syntax = "proto3";
package backup.v1;

import "google/protobuf/timestamp.proto";

// ============================================================
// Main service
// ============================================================
service BackupService {
    // Agent registers itself (before approval)
    rpc Register(RegisterRequest) returns (RegisterResponse);

    // Bidirectional stream — agent's persistent connection
    // Agent sends heartbeats & status, server sends commands & config
    rpc Connect(stream AgentMessage) returns (stream ServerMessage);

    // Agent reports a completed job
    rpc ReportJob(JobReport) returns (JobReportAck);

    // Agent reports current snapshots for its repos
    rpc ReportSnapshots(SnapshotReport) returns (SnapshotReportAck);
}

// ============================================================
// Enrollment
// ============================================================
message RegisterRequest {
    string hostname = 1;
    string os = 2;                    // e.g. "linux/amd64"
    string agent_version = 3;
    string restic_version = 4;
    string rclone_version = 5;
}

message RegisterResponse {
    string agent_id = 1;
    AgentStatus status = 2;           // PENDING on first registration
}

enum AgentStatus {
    AGENT_STATUS_UNSPECIFIED = 0;
    AGENT_STATUS_PENDING = 1;
    AGENT_STATUS_APPROVED = 2;
    AGENT_STATUS_REJECTED = 3;
}

// ============================================================
// Bidirectional Connect stream
// ============================================================

// Agent -> Server messages
message AgentMessage {
    string agent_id = 1;
    string api_key = 2;               // auth (empty during pending state)

    oneof payload {
        Heartbeat heartbeat = 10;
        ConfigAck config_ack = 11;
        CommandResult command_result = 12;
    }
}

message Heartbeat {
    google.protobuf.Timestamp timestamp = 1;
    string status = 2;                // "idle", "running", "degraded"
    RunningJob current_job = 3;       // null if idle
    string agent_version = 4;
    string restic_version = 5;
    string rclone_version = 6;
}

message RunningJob {
    string plan_name = 1;
    google.protobuf.Timestamp started_at = 2;
    float progress_percent = 3;       // 0-100 if available, -1 if unknown
}

message ConfigAck {
    int32 config_version = 1;
    bool success = 2;
    string error = 3;                 // set if success=false
}

message CommandResult {
    string command_id = 1;            // echoes the command ID from server
    bool success = 2;
    string error = 3;
    bytes data = 4;                   // JSON payload for queries (snapshots, file listings)
}

// Server -> Agent messages
message ServerMessage {
    oneof payload {
        Approval approval = 10;
        AgentConfig config = 11;
        Command command = 12;
    }
}

message Approval {
    AgentStatus status = 1;           // APPROVED or REJECTED
    string api_key = 2;              // set on approval
}

message AgentConfig {
    int32 config_version = 1;
    repeated Repository repositories = 2;
    repeated BackupPlan backup_plans = 3;
    RetentionPolicy default_retention = 4;
    string rclone_config = 5;         // raw INI text
    // No global hooks — hooks are composed per plan (scripts resolved server-side)
}

message Command {
    string command_id = 1;            // unique ID for correlating results

    oneof action {
        TriggerBackup trigger_backup = 10;
        TriggerRestore trigger_restore = 11;
        ListSnapshots list_snapshots = 12;
        BrowseSnapshot browse_snapshot = 13;
    }
}

message TriggerBackup {
    string plan_id = 1;
}

message TriggerRestore {
    string repository_id = 1;
    string snapshot_id = 2;
    repeated string paths = 3;        // specific paths to restore, empty = all
    string target = 4;                // restore target dir (e.g. "/mnt/restore")
}

message ListSnapshots {
    string repository_id = 1;
}

message BrowseSnapshot {
    string repository_id = 1;
    string snapshot_id = 2;
    string path = 3;                  // directory to list within snapshot
}

// ============================================================
// Shared types
// ============================================================
message Repository {
    string id = 1;
    string name = 2;
    string type = 3;
    string path = 4;
    string password = 5;
}

message BackupPlan {
    string id = 1;
    string name = 2;
    repeated string paths = 3;
    repeated string excludes = 4;
    repeated string tags = 5;
    repeated string repository_ids = 6;
    string schedule = 7;
    bool forget_after_backup = 8;
    bool prune_after_forget = 9;
    string prune_schedule = 10;
    RetentionPolicy retention = 11;   // null = use default
    repeated ResolvedHook hooks = 12; // scripts resolved to commands by server
    bool enabled = 13;
}

message RetentionPolicy {
    int32 keep_last = 1;
    int32 keep_hourly = 2;
    int32 keep_daily = 3;
    int32 keep_weekly = 4;
    int32 keep_monthly = 5;
    int32 keep_yearly = 6;
}

// Hook as seen by the agent — scripts are resolved to commands by the server.
// The agent doesn't know about scripts vs inline, it just executes commands.
message ResolvedHook {
    string name = 1;                  // for logging/reporting: "dump-postgres", "healthcheck-start"
    string on_event = 2;             // "pre_backup", "post_backup", "on_success", "on_failure"
    int32 sort_order = 3;
    string type = 4;                  // "command"
    string command = 5;               // resolved command to execute
    int32 timeout_seconds = 6;        // resolved timeout
    string on_error = 7;              // "abort", "continue"
}

// ============================================================
// Job reporting
// ============================================================
message JobReport {
    string agent_id = 1;
    string api_key = 2;

    string job_id = 3;
    string plan_id = 4;
    string plan_name = 5;
    string type = 6;                  // "backup", "forget", "prune", "restore"
    string trigger = 7;               // "scheduled", "manual"
    string status = 8;                // "success", "partial", "failed"

    google.protobuf.Timestamp started_at = 9;
    google.protobuf.Timestamp finished_at = 10;

    repeated RepositoryResult repository_results = 11;
    repeated HookResult hook_results = 12;

    string log_tail = 13;
}

message RepositoryResult {
    string repository_id = 1;
    string repository_name = 2;
    string status = 3;                // "success", "failed", "skipped"
    string snapshot_id = 4;
    string error = 5;

    int64 files_new = 6;
    int64 files_changed = 7;
    int64 files_unmodified = 8;
    int64 bytes_added = 9;
    int64 total_bytes = 10;
    int64 duration_ms = 11;
}

message HookResult {
    string hook_name = 1;
    string phase = 2;
    string status = 3;
    string error = 4;
    int64 duration_ms = 5;
}

message JobReportAck {
    bool success = 1;
    string error = 2;
}

// ============================================================
// Snapshot reporting
// ============================================================
message SnapshotReport {
    string agent_id = 1;
    string api_key = 2;
    string repository_id = 3;
    repeated SnapshotInfo snapshots = 4;
}

message SnapshotInfo {
    string id = 1;                    // short ID
    string long_id = 2;              // full ID
    google.protobuf.Timestamp time = 3;
    string hostname = 4;
    repeated string tags = 5;
    repeated string paths = 6;
}

message SnapshotReportAck {
    bool success = 1;
}
```

## Connection Lifecycle

```
Agent starts
  │
  ├─ Has identity.yaml?
  │   NO ──> Register() ──> gets agent_id, status=PENDING
  │   YES ─> skip registration
  │
  ├─ Connect() stream opens
  │   │
  │   ├─ If PENDING: send heartbeats, wait for Approval message
  │   │   └─ On Approval: store api_key, include in all future messages
  │   │
  │   ├─ If APPROVED: send heartbeats with api_key
  │   │   ├─ Receive AgentConfig ──> persist locally, ACK
  │   │   ├─ Receive Command ──> execute, send CommandResult
  │   │   └─ Loop
  │   │
  │   └─ If REJECTED: log and shut down (or retry after long delay)
  │
  ├─ On disconnect: reconnect with exponential backoff
  │   (1s, 2s, 4s, 8s, ... max 5min)
  │
  └─ On backup job completion: ReportJob() unary RPC
```

## Server REST API (for Web UI)

The Vue.js frontend talks to the Go server over REST. This is a separate API from the gRPC agent API.

```
# Agents
GET    /api/agents                  List all agents
GET    /api/agents/:id              Get agent details
POST   /api/agents/:id/approve      Approve pending agent
POST   /api/agents/:id/reject       Reject pending agent
DELETE /api/agents/:id              Remove agent
PUT    /api/agents/:id/rclone       Update agent's rclone config

# Repositories
GET    /api/repositories            List all repositories (filterable: ?scope=global, ?agent_id=X)
POST   /api/repositories            Create repository (scope + optional agent_id in body)
GET    /api/repositories/:id        Get repository details
PUT    /api/repositories/:id        Update repository
DELETE /api/repositories/:id        Delete repository

# Scripts (reusable hook definitions)
GET    /api/scripts                 List all scripts
POST   /api/scripts                 Create script
GET    /api/scripts/:id             Get script details
PUT    /api/scripts/:id             Update script
DELETE /api/scripts/:id             Delete script (fails if referenced by plan hooks)

# Backup Plans
GET    /api/plans                   List all plans (filterable by agent)
POST   /api/plans                   Create plan
GET    /api/plans/:id               Get plan details
PUT    /api/plans/:id               Update plan
DELETE /api/plans/:id               Delete plan
POST   /api/plans/:id/trigger       Trigger immediate backup

# Plan Hooks (ordered list per plan)
GET    /api/plans/:id/hooks         List hooks for plan (ordered)
POST   /api/plans/:id/hooks         Add hook (script ref or inline command)
PUT    /api/plans/:id/hooks/:hid    Update hook
DELETE /api/plans/:id/hooks/:hid    Remove hook
PUT    /api/plans/:id/hooks/reorder Reorder hooks

# Jobs
GET    /api/jobs                    List jobs (filterable by agent, plan, status)
GET    /api/jobs/:id                Get job details with results

# Snapshots
GET    /api/agents/:id/snapshots?repo=:repo_id    List snapshots
POST   /api/agents/:id/snapshots/browse            Browse snapshot files
POST   /api/agents/:id/restore                     Trigger restore

# Settings
GET    /api/settings                Get global settings
PUT    /api/settings                Update global settings (retention defaults, etc.)
```
