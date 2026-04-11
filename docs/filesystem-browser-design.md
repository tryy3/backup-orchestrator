# Remote Filesystem Browser Design

Feature: allow users to browse the live filesystem of a connected agent directly
from the UI, so they can pick source paths when configuring repositories and backup
plans without having to manually type them.

Tracked in: [Issue #8](https://github.com/tryy3/backup-orchestrator/issues/8)

---

## Problem

When configuring a repository root path or a backup plan source path, the user must
type the full absolute path by hand. This is error-prone and requires prior knowledge
of the agent's directory layout. There is no way to discover or validate paths from
the UI.

This is different from the existing `BrowseSnapshot` command (which lists files
**inside a restic snapshot**). The filesystem browser operates on the **live agent
filesystem** via `os.ReadDir`.

---

## Design Decisions

### On-demand, per-directory listing (not a full tree)

Each request returns only the **immediate children** of one directory. The user
navigates one level at a time. No recursive tree is sent.

**Rationale:** source directories can contain thousands of files. Sending a full tree
upfront would be large, slow, and mostly unused — users typically know the rough area
they want and navigate only a few levels.

### Request/response over the existing gRPC stream

The request goes through the normal `Command` / `CommandResult` flow:

```
Frontend
  └─ POST /api/agents/:id/fs?path=/home
       └─ Server: SendCommand (BrowseFilesystem, path=/home)
            └─ Stream → Agent
                 └─ os.ReadDir("/home")
                      └─ CommandResult { data: JSON[] }
            └─ Server: returns JSON to REST caller
       └─ Frontend renders directory listing
```

No new gRPC methods or streaming primitives are needed. The 30-second command timeout
is ample for a single `os.ReadDir` call.

### No server-side caching

The filesystem changes (new mounts, created directories). Since browsing is
interactive and infrequent, each navigation sends a fresh command to the agent.
Client-side caching per session (within the UI component) is fine as a navigation
aid, but should not be persisted.

### Agents that are offline fail fast

`SendCommand` returns immediately with "agent disconnected" if the agent is not
connected. The UI should surface this clearly rather than waiting.

---

## Communication Protocol

### New proto message: `BrowseFilesystem`

Added to the existing `Command.oneof action`:

```protobuf
message BrowseFilesystem {
    string path = 1;   // absolute path to list; defaults to "/" if empty
}
```

Agent response: `CommandResult.data` is a JSON array of `FilesystemEntry`.

### Response payload: `FilesystemEntry`

```json
[
  { "name": "home", "path": "/home" },
  { "name": "etc",  "path": "/etc"  }
]
```

The agent filters to **directories only** before marshalling the response — files are
never included. Only `name` and `path` are returned; no permissions, no symlink
targets, no inode numbers, no sizes.

### REST endpoint

```
GET /api/agents/:id/fs?path=<absolute-path>
```

- Returns `200 OK` with `FilesystemEntry[]` JSON on success.
- Returns `502 Bad Gateway` if agent is not connected.
- Returns `400 Bad Request` if path is missing or fails validation.
- Returns `500 Internal Server Error` if the agent returns an error (e.g. permission denied).

---

## Security Considerations

The agent handler **must** apply these guards before calling `os.ReadDir`:

1. **Absolute path required** — reject any relative path or empty string (default to `/`
   only if the caller explicitly omits the field, so the UI can always show the root).
2. **No path traversal** — clean the path with `filepath.Clean` and verify it still starts
   with `/` and contains no `..` components after cleaning.
3. **Blocked prefixes** — refuse to list `/proc`, `/sys`, `/dev`, `/run/credentials`,
   and similar pseudo/sensitive mounts. Return a clear error so the UI can display it.
4. **Entries only, no content** — the handler calls `os.ReadDir` only; it never opens,
   reads, or stats individual files beyond what `DirEntry` provides.
5. **Error passthrough** — `os.ReadDir` permission errors are returned as a failed
   `CommandResult`, not silently swallowed. The user sees "permission denied" rather
   than an empty listing.

---

## Implementation Plan

### 1. Proto (`proto/backup/v1/backup.proto`)

Add `BrowseFilesystem` message and a new `browse_filesystem` field to the `Command`
oneof. Regenerate with `just proto-gen`.

### 2. Agent (`agent/`)

- Add a `BrowseFilesystem` case to `handleCommand` in `cmd/agent/main.go`.
- Implement path validation (clean, block-list) before calling `os.ReadDir`.
- Filter `os.ReadDir` results to `IsDir() == true` before marshalling.
- Marshal result to `[]FilesystemEntry` JSON and set `CommandResult.Data`.
- Define `FilesystemEntry` struct (`Name`, `Path` string fields) in `internal/executor/` (or inline in main).

### 3. Server REST (`server/internal/api/`)

- Add `browseFilesystemHandler` in `snapshots.go` (or a new `filesystem.go`).
- Register `GET /agents/{id}/fs` on the router in `router.go`.
- Forward `path` query parameter as-is; path validation belongs to the agent.

### 4. Frontend

- **API client** (`src/api/client.ts`): add `agents.browseFs(agentId, path)`.
- **Types** (`src/types/api.ts`): add `FilesystemEntry` type (`name: string; path: string`).
- **Component** (`src/components/common/FileBrowser.vue`): inline dropdown tree panel
  (see _UI Design_ section below for full spec).
- **Integration**: use the component in repository path inputs and backup plan source
  path inputs where an `agent_id` is known.

---

## UI Design

Two prototypes were produced in Stitch (`docs/file-browser/`). The **inline tree
dropdown** (`stitch_tree_root.png` / `stitch_tree_nested.png`) is the chosen
approach. The terminal-style modal (`stitch_nested_nav.png`) is kept as reference for
the keyboard-navigation behaviour.

### Chosen design: inline tree dropdown

**Trigger area** — an editable text input with a folder icon on the left and a
**Browse** button on the right.

```
[ 📁  /server/backups/                      ] [ Browse ]
```

When the user has navigated deep, the input truncates with a `…` prefix:

```
[ 📁  … /server/media/archives              ] [ Browse ]
```

The input is **directly editable** — users who know the path can type it in and press
Enter to jump straight to that directory in the tree (or to commit the value without
opening the panel). Navigating the tree updates the input live.

The folder icon tint changes to indicate an active/selected state (e.g. a
terracotta/warning colour when a path is selected but not yet confirmed).

**Dropdown panel** — appears directly below the trigger, labelled **Select
Destination**. It shows an expandable tree rooted at `/`. Only directories are
shown (the agent never returns files). The panel has a fixed max-height (approx.
320 px / ~10 rows) with vertical overflow scrolling so deep trees remain usable.
The root contents are fetched eagerly when the panel opens — no manual expansion of
a root node is required.

Root state (`stitch_tree_root.png`):

```
Select Destination
  > 📁 server
  > 📁 media
  > 📁 archives
  > 📁 system
  > 📁 users
```

Expanded/nested state (`stitch_tree_nested.png`):

```
Select Destination
  v 📁 server
      > 📁 backups
  v 📁 media
    v 📁 archives          ✓   ← selected
        > 📁 images
        > 📂 videos
  > 📁 system
  > 📁 local
```

- Each row has a `>` chevron (collapsed) or `v` chevron (expanded) and a folder
  icon.
- Clicking a chevron fetches that directory's children (one `GET /api/agents/:id/fs`
  call) and expands the node inline.
- Clicking the folder name or row selects that path — shown with a checkmark on the
  right and updates the trigger input immediately.
- Already-fetched children are cached in component state for the duration of the
  browse session so re-expanding a node does not re-fetch.
- A per-node loading spinner replaces the chevron while the fetch is in flight.
- An error state per node (e.g. "permission denied") is shown inline below the node
  label.

**Closing the panel** — clicking outside the panel, pressing Esc, or clicking Browse
again closes it without changing the committed path.

**Confirming the selection** — the path displayed in the trigger input updates live as
the user clicks nodes. The panel closes (and the value is emitted to the parent) when
the user clicks outside or explicitly clicks a "Select" / confirm affordance if one is
added.

### Reference design: terminal-style modal (`stitch_nested_nav.png`)

This prototype demonstrates the desired keyboard-navigation model regardless of which
visual shell is used:

- **Header bar**: current path with a folder icon (e.g. `/var/www/`).
- **Entry list**: `.. (Parent Directory)` always first; then directory entries with
  folder icons.
- **Highlighted row**: the focused entry is highlighted in a distinct accent colour
  (cyan/teal in the prototype).
- **Hint text** on the focused row: `← to select, Tab to enter`.
- **Bottom status bar**: `Navigate: ↑↓` on the left, `Cancel: Esc` on the right.

Even in the chosen inline tree design, the component should support full keyboard
navigation:

| Key | Action |
|-----|--------|
| `↑` / `↓` | Move focus between visible rows |
| `→` / `Tab` | Expand the focused directory (fetch children if needed) |
| `←` | Collapse the focused directory |
| `Enter` | Select the focused path and close the panel |
| `Esc` | Close the panel without changing the path |

---

## Out of Scope (this issue)

- Browsing the filesystem of agents that are **not** the one being configured.
- Selecting **multiple** paths at once from one browse session (can be a follow-up).
- Showing file sizes or last-modified timestamps in the listing (not needed for path
  picking).
- Autocomplete/fuzzy search across the filesystem (alternative solution from the issue;
  deferred).
