# Architecture Overview

## System Components

```
+--------------------------------------------------+
|                   Server                          |
|                                                   |
|  +------------+  +------------+  +-----------+    |
|  |  Web UI    |  | Config     |  | Scheduler |    |
|  |  (SPA)     |  | Manager    |  | (Central) |    |
|  +-----+------+  +-----+------+  +-----+-----+   |
|        |               |               |          |
|  +-----+---------------+---------------+------+   |
|  |              Core Server API               |   |
|  +-----+------------------+-------------------+   |
|        |                  |                       |
|  +-----+------+    +-----+------+                 |
|  | Repository  |    | Agent      |                |
|  | Registry    |    | Manager    |                |
|  +-------------+    +-----+------+                |
|                           |                       |
+--------------------------------------------------+
                            |
              gRPC / REST (TLS + mTLS)
                            |
        +-------------------+-------------------+
        |                   |                   |
+-------+------+    +-------+------+    +-------+------+
|   Agent A    |    |   Agent B    |    |   Agent C    |
|   (host-1)   |    |   (host-2)   |    |   (host-3)   |
|              |    |              |    |              |
| - Local cfg  |    | - Local cfg  |    | - Local cfg  |
| - Scheduler  |    | - Scheduler  |    | - Scheduler  |
| - Restic     |    | - Restic     |    | - Restic     |
| - Hook exec  |    | - Hook exec  |    | - Hook exec  |
| - Status rpt |    | - Status rpt |    | - Status rpt |
+--------------+    +--------------+    +--------------+
```

## Component Responsibilities

### Server
- **Web UI**: Configure backups, repositories, hosts. Monitor status, browse snapshots, trigger restores.
- **Config Manager**: Stores the "source of truth" configuration. Pushes config down to agents when changed.
- **Repository Registry**: Global repository definitions (S3, local, SFTP, etc.) with credentials. Agents receive only the repos they need.
- **Agent Manager**: Tracks agent connectivity, health, and status. Handles agent registration/enrollment.
- **Scheduler (Central)**: Optional — knows about all schedules for the dashboard view. The actual scheduling runs on agents.

### Agent
- **Local Config**: Persisted copy of the configuration received from the server. Survives server downtime.
- **Scheduler**: Runs cron-style schedules locally. Completely independent of server availability.
- **Restic Executor**: Wraps restic CLI calls (backup, restore, forget, prune, copy, snapshots, etc.).
- **Hook Executor**: Runs pre/post hooks at each lifecycle point.
- **Status Reporter**: Pushes backup results, logs, and health to the server (with buffering if server is unreachable).

## Data Flow

### Configuration Flow (Server -> Agent)
1. User configures backup plan via Web UI
2. Server validates and stores config
3. Server pushes config to the relevant agent(s)
4. Agent persists config locally and updates its scheduler
5. Agent ACKs the config version

### Backup Flow (Agent-local)
1. Scheduler triggers a backup job
2. Pre-backup hooks execute (e.g., dump databases, stop services)
3. Restic backup runs against configured repository/repositories
4. Post-backup hooks execute (e.g., notifications, cleanup)
5. Agent records result (success/failure, stats, duration, snapshot ID)

### Status Flow (Agent -> Server)
1. After each backup job, agent pushes a status report to the server
2. If server is unreachable, agent buffers reports locally
3. When connectivity resumes, agent sends buffered reports
4. Server also periodically pings agents for health checks (heartbeat)

### Restore Flow (Server -> Agent -> User)
1. User requests restore via Web UI (selects host, snapshot, paths)
2. Server sends restore command to agent
3. Agent executes `restic restore` with the specified parameters
4. Agent streams progress back to server for display in UI

## Key Design Principles

1. **Agent independence**: Agents must function fully without the server. Config is local, scheduling is local.
2. **Server as control plane**: Server is for configuration, monitoring, and on-demand operations — not in the critical backup path.
3. **Restic as the engine**: We wrap restic, not reimplement it. All backup/restore operations are restic CLI calls.
4. **Secure by default**: Agent-server communication uses TLS. Agent enrollment uses a token-based handshake.
