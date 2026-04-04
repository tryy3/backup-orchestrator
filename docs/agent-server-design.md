# Agent-Server Communication Design

## Decisions Made

- **Connection direction**: Agent connects to server (outbound only)
- **Protocol**: gRPC (bidirectional streaming)
- **Agent bootstrap**: Environment variables
- **Networking**: Tailscale assumed (but should work without it)
- **Enrollment**: Auto-approval (agent connects, admin approves in dashboard)

---

## Connection Model

Agents initiate outbound connections to the server. The server is the only component that needs open ports.

```
Agent A ---> Server (:8443 gRPC, :8080 Web UI)
Agent B --->
Agent C --->
```

With Tailscale, agents reach the server via MagicDNS (e.g., `backup-server.tail1234.ts.net`). Without Tailscale, the server needs a reachable IP/hostname.

## Agent Bootstrap

Agents are configured at first start using environment variables:

```bash
# Minimal bootstrap — just point at the server
BACKUP_SERVER_URL=backup-server.tail1234.ts.net:8443

# Optional overrides
BACKUP_AGENT_NAME=webserver-01       # defaults to hostname
BACKUP_DATA_DIR=/var/lib/backup-orchestrator
```

That's it. No tokens to copy around. Start the agent, then go to the dashboard to approve it.

### Example systemd unit

```ini
[Unit]
Description=Backup Orchestrator Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/etc/backup-orchestrator/agent.env
ExecStart=/usr/local/bin/backup-agent
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Where `/etc/backup-orchestrator/agent.env`:
```bash
BACKUP_SERVER_URL=backup-server.tail1234.ts.net:8443
```

## Communication Protocol: gRPC

gRPC with bidirectional streaming. Agent connects outbound, server can push commands and config updates back through the same connection.

```protobuf
// Simplified — actual proto will be more detailed
service BackupService {
  // Agent enrollment
  rpc Enroll(EnrollRequest) returns (EnrollResponse);

  // Bidirectional stream — agent connects, server pushes commands
  rpc Connect(stream AgentMessage) returns (stream ServerMessage);

  // Agent reports job results
  rpc ReportJob(JobReport) returns (JobReportAck);
}
```

**Why gRPC over alternatives:**
- Strongly typed contracts (protobuf)
- Bidirectional streaming in one connection
- Efficient binary protocol
- Good Go ecosystem

Can revisit later if we need REST for external integrations (e.g., a REST gateway in front of gRPC).

## Enrollment (Auto-Approval)

The workflow is: configure the agent, start it, then approve it in the dashboard.

### Flow

```
1. Admin sets up agent on host
   - Install binary, configure systemd with BACKUP_SERVER_URL
   - Start the agent service

2. Agent connects and registers as "pending"
   Agent -> Server: { hostname, os, restic_version, agent_version }
   Server -> Agent: { agent_id, status: "pending" }

3. Agent enters waiting state
   - Keeps connection open, sends heartbeats
   - Does NOT run any backups yet
   - Retries connection with backoff if server is unreachable

4. Admin sees new agent in Web UI dashboard
   Dashboard shows: "webserver-01 (pending approval)"
   Admin clicks "Approve"

5. Server approves and issues API key
   Server -> Agent (over existing connection): { status: "approved", api_key: "..." }

6. Agent stores identity locally and is now active
   Writes agent_id + api_key to identity.yaml
   Ready to receive config and run backups

7. Admin configures backup plans for this agent in the Web UI
```

### Agent States

| State       | Description                                      | Runs Backups? |
|-------------|--------------------------------------------------|---------------|
| `pending`   | Connected to server, waiting for admin approval   | No            |
| `approved`  | Approved, has API key, ready for config            | Yes           |
| `active`    | Has config, running scheduled backups              | Yes           |
| `rejected`  | Admin rejected enrollment (agent stops connecting) | No            |
| `offline`   | Approved but not currently connected               | Yes (locally) |

### Security Notes

- Tailscale provides encrypted transport and machine-level auth at the network layer
- On top of that, each approved agent gets an API key for app-level auth
- The server can revoke an agent by deleting it — agent's API key becomes invalid
- An unapproved agent can never receive config or repo credentials

## Communication Patterns

### 1. Persistent Connection (Agent -> Server)

After enrollment, the agent opens a bidirectional gRPC stream (`Connect` RPC). This stream stays open and is used for:

- Agent -> Server: heartbeats, status updates
- Server -> Agent: config pushes, on-demand commands

If the connection drops, the agent reconnects with exponential backoff.

### 2. Heartbeat (Agent -> Server, periodic)

```
Every 30-60 seconds, over the Connect stream:
Agent -> Server: {
  agent_id,
  timestamp,
  status: "idle" | "running" | "degraded",
  current_job: null | { plan_name, started_at, progress_pct },
  restic_version,
  agent_version
}
```

Server marks agent as "unreachable" in UI if no heartbeat for 3 intervals.

### 3. Config Push (Server -> Agent, on change)

```
Over the Connect stream:
Server -> Agent: {
  config_version: 42,
  backup_plans: [...],
  repositories: [...],
  hooks: [...],
  retention_policies: [...]
}

Agent -> Server: { config_version: 42, status: "applied" | "error", error: "..." }
```

Agent persists config locally. On startup, loads local config and checks server for updates.

### 4. Job Report (Agent -> Server, after each job)

Sent as a separate unary RPC (not over the stream) so it's reliable even if the stream momentarily disconnects:

```
Agent -> Server: {
  agent_id,
  job_id,
  plan_name,
  type: "backup" | "forget" | "prune" | "restore",
  status: "success" | "partial" | "failed",
  started_at,
  finished_at,
  repositories: [
    { name: "local-nas", status: "success", snapshot_id: "abc123", stats: {...} },
    { name: "s3-primary", status: "failed", error: "connection timeout" }
  ],
  hooks: [
    { name: "dump-postgres", phase: "pre", status: "success", duration_ms: 3200 },
    { name: "notify-slack", phase: "post", status: "success", duration_ms: 150 }
  ],
  log_tail: "..."
}
```

If server is unreachable, buffer locally in SQLite and replay when connected.

### 5. On-Demand Commands (Server -> Agent)

Sent over the Connect stream:

- **Trigger backup now**: Run a specific backup plan immediately
- **Restore**: Restore files from a snapshot
- **List snapshots**: Query restic snapshots for a repo
- **Browse snapshot**: List files in a snapshot (for UI file browser)
- **Update config**: Push new configuration

## Offline Resilience

The agent must function fully without the server:

| Scenario | Agent Behavior |
|----------|---------------|
| Server unreachable at startup | Load last-known local config, start scheduler |
| Server goes down mid-operation | Complete current job, buffer status report |
| Server unreachable for heartbeat | Continue operating, retry connection with backoff |
| Server unreachable for job report | Buffer in local SQLite, replay on reconnect |
| Server never comes back | Agent runs indefinitely on last-known config |

## Container Deployment (Agent)

Agents run in containers with broad read-only mounts. Backup paths are configured centrally via the server — no container restart needed when adding new backup plans.

### Volume Strategy

Mount host paths under `/mnt/backup/` read-only. Backup plan paths in the config reference this prefix.

```yaml
# docker-compose.yml
services:
  backup-agent:
    image: backup-orchestrator/agent:latest
    restart: unless-stopped
    environment:
      BACKUP_SERVER_URL: backup-server.tail1234.ts.net:8443
      BACKUP_AGENT_NAME: webserver-01
    volumes:
      # Backup sources — read-only
      - /home:/mnt/backup/home:ro
      - /var:/mnt/backup/var:ro
      - /etc:/mnt/backup/etc:ro
      # Restore target — read-write
      - /var/lib/backup-restore:/mnt/restore:rw
      # Agent state — persists across container restarts
      - backup-agent-data:/var/lib/backup-orchestrator

volumes:
  backup-agent-data:
```

Then a backup plan configured in the UI would target `/mnt/backup/home` instead of `/home`. The server knows about this prefix per agent.

### What's in the container image

- Agent binary
- Restic binary (pinned version)
- Minimal base image (Alpine or distroless)

### Restore

The `/mnt/restore` volume is the one writable mount. Restores go there, and the admin moves files into place on the host. This avoids needing write access to the actual data volumes.

## Local State (Agent)

Inside the container, persisted via the `backup-agent-data` volume:

```
/var/lib/backup-orchestrator/agent/
  identity.yaml           # agent_id, api_key (written at enrollment)
  config.yaml             # last-known config from server
  state.db                # SQLite: job history, buffered reports, scheduler state
  logs/                   # per-job structured logs
```
