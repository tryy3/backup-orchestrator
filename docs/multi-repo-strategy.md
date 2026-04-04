# Multi-Repository Strategy

## Decision

**Independent backups (Option A)** — run `restic backup` separately for each target repository.

```
Filesystem --> restic backup --> Repo A (local NAS)
Filesystem --> restic backup --> Repo B (S3 via rclone)
Filesystem --> restic backup --> Repo C (B2 via rclone)
```

### Why

- Simple to implement — same backup command, different `--repo` flag
- Better fault isolation — corruption in one repo doesn't propagate
- Flexible per backup plan — some plans back up to 1 repo, others to all 3
- Partial failure is straightforward (repo B fails, A and C succeed)
- No requirement for matching chunker params or simultaneous repo access

### Future consideration

If `restic copy` becomes useful later (e.g., large datasets where re-reading the filesystem is expensive), the config model can be extended with a `multi_repo_strategy` field per backup plan. But for MVP and likely well beyond, independent backups are the way to go.

## Repository Configuration Model

Repositories are defined globally on the server. Each backup plan references which repos it targets by name.

```yaml
repositories:
  - name: "local-nas"
    type: "local"
    path: "/mnt/nas/backups"
    password: "..."  # managed by server, encrypted at rest

  - name: "s3-offsite"
    type: "rclone"
    rclone_remote: "s3-backup"    # references rclone config name
    rclone_path: "my-bucket/backups"
    password: "..."

  - name: "b2-offsite"
    type: "rclone"
    rclone_remote: "b2-backup"
    rclone_path: "offsite-bucket/backups"
    password: "..."

backup_plans:
  - name: "daily-home"
    paths: ["/mnt/backup/home"]
    exclude: ["*.tmp", ".cache"]
    repositories: ["local-nas", "s3-offsite"]   # backs up to both
    schedule: "0 2 * * *"

  - name: "database"
    paths: ["/mnt/backup/var/lib/postgres/dump"]
    repositories: ["local-nas", "s3-offsite", "b2-offsite"]  # all three
    schedule: "0 */6 * * *"

  - name: "media-weekly"
    paths: ["/mnt/backup/home/media"]
    repositories: ["local-nas"]   # local only, no need for offsite
    schedule: "0 3 * * 0"
```

## rclone Integration

Many repository backends are configured through rclone (protocols restic doesn't natively support). For MVP, rclone is a first-class citizen alongside restic.

### Container image includes

- `restic` binary
- `rclone` binary
- Agent binary

### rclone config management

The rclone config is managed per-agent through the server dashboard. For MVP, this is a raw text input — paste your rclone config and the server pushes it to the agent.

```
Server Web UI:
  Agent "webserver-01" > rclone Config
  +------------------------------------------+
  | [s3-backup]                              |
  | type = s3                                |
  | provider = AWS                           |
  | access_key_id = AKIA...                  |
  | secret_access_key = ...                  |
  | region = eu-west-1                       |
  |                                          |
  | [b2-backup]                              |
  | type = b2                                |
  | account = ...                            |
  | key = ...                                |
  +------------------------------------------+
  [Save & Push to Agent]
```

The agent stores this as `/var/lib/backup-orchestrator/agent/rclone.conf` and restic uses it via `--option rclone.config=...` or the `RCLONE_CONFIG` env var.

### Future improvements (out of scope for MVP)

- Wizard UI for configuring rclone remotes (dropdown for provider, form fields per provider)
- Test connection button (verify rclone remote is reachable)
- rclone config shared across agents (define once, assign to many)

## How the Agent Executes Multi-Repo Backups

When a backup plan targets multiple repositories, the agent runs them sequentially:

```
1. Run pre_backup hooks (once)
2. For each repository in plan.repositories:
   a. restic backup --repo <repo> --paths <paths>
   b. Record result (success/failure per repo)
3. Run post_backup hooks (once)
4. Run on_success or on_failure hooks based on aggregate result
5. Report all results to server
```

Hooks run once per job, not once per repository. The job status is:
- `success` — all repos succeeded
- `partial` — some repos succeeded, some failed
- `failed` — all repos failed
