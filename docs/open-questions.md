# Decisions & Open Questions

## Decided

### 1. Language: Go
- Restic is written in Go — potential to use as a library in the future
- Single binary for both server and agent
- Strong gRPC/protobuf ecosystem
- Cross-compilation for different platforms

### 2. Web UI: Vue.js SPA + Go API backend
- Vue.js frontend (SPA)
- Go backend serves API + static frontend assets
- Rich, responsive dashboard for configuration and monitoring

### 3. Database: SQLite
- Zero-dependency, single file, perfect for single-server deployment
- Good enough for expected scale (< 100 agents)
- Can add PostgreSQL support later if needed

### 4. Credentials: Plain text in config (MVP)
- Credentials stored directly in the server database alongside config
- Pushed to agents as part of config over Tailscale-secured connection
- Agent stores them in local config file

**Roadmap for credentials:**
1. MVP: plain text in config (acceptable given Tailscale + controlled environment)
2. Near future: `password-command` support (delegate to external tools like `pass`, `vault`)
3. Later: built-in integration with HashiCorp Vault or similar

### 5. Distribution: Docker containers
- Agent: container image with agent binary + restic binary + rclone binary
- Server: container image with server binary + embedded Vue.js frontend
- Docker Compose for easy deployment

### 6. Retention Policies: Global defaults + per-plan overrides

Follows the autorestic pattern — define retention globally, override per plan when needed.

```yaml
# Global retention defaults (apply to all plans unless overridden)
retention:
  keep_last: 7
  keep_daily: 30
  keep_weekly: 12
  keep_monthly: 24

backup_plans:
  - name: "daily-home"
    schedule: "0 2 * * *"
    # uses global retention defaults

  - name: "database"
    schedule: "0 */6 * * *"
    retention:                    # overrides global
      keep_last: 14
      keep_daily: 60
      keep_weekly: 24
      keep_monthly: 48
```

**Forget + prune behavior:**
- Default: runs after each backup (current workflow, works well)
- Configurable: can be switched to a separate schedule per plan if prune becomes slow

```yaml
backup_plans:
  - name: "daily-home"
    schedule: "0 2 * * *"
    forget_after_backup: true        # default: true
    prune_after_forget: true         # default: true

  - name: "large-archive"
    schedule: "0 3 * * 0"
    forget_after_backup: true
    prune_after_forget: false        # prune separately (it's slow for large repos)
    prune_schedule: "0 4 * * 0"     # prune on its own schedule
```

### 7. Project Name: `backup-orchestrator`

Keeping for now. May rename later.

### 8. Scope for MVP (v1)

#### v1 (MVP)
- [ ] Agent + Server with gRPC communication
- [ ] Agent enrollment (auto-approval via dashboard)
- [ ] Server Web UI (Vue.js): configure repos, rclone config, backup plans, scripts, hooks, view status
- [ ] Repositories with scope (local per-agent + global shared)
- [ ] Agent: run scheduled backups (independent multi-repo strategy)
- [ ] rclone integration (binary in container, raw config via dashboard)
- [ ] Scripts (reusable) + plan hooks (composition model, command type only)
- [ ] Retention with forget+prune (global defaults + per-plan overrides)
- [ ] Automatic snapshot tags (agent, plan, trigger)
- [ ] Job history and status reporting
- [ ] Basic restore to `/mnt/restore` via UI

#### v2
- [ ] Custom tags at global/agent/repository levels
- [ ] Webhook / notification hook types for scripts
- [ ] Snapshot browser (browse/download files in UI)
- [ ] `password-command` support for credentials
- [ ] Copy-based multi-repo strategy (opt-in per plan)
- [ ] rclone config wizard UI
- [ ] Coordinated prune scheduling for shared repos

#### v3
- [ ] Multi-user auth on Web UI
- [ ] Role-based access control
- [ ] Audit logging
- [ ] Metrics export (Prometheus)
- [ ] Vault / external secret store integration
