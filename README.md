# devhub

A single-binary terminal hub for macOS. It owns an isolated tmux server (`-L devhub`), a local daemon/API, and a Bubble Tea UI so every project terminal, background process, skill, and port lives in one place.

CLI and TUI talk to the daemon over `~/.devhub/devhub.sock` when it is healthy, and fall back to in-process `internal/core`.

## Install

Requirements:

- Go 1.22+
- tmux 3.3+ (`brew install tmux`) — needed for `display-popup`

```bash
git clone https://github.com/rushikesh/devhub
cd devhub
make install          # ~/.local/bin/devhub, or ~/bin if that prefix is not writable
# or: make install PREFIX=/usr/local
```

`make install` also creates a `dev -> devhub` symlink in the same directory. If `dev` already exists on PATH (another binary, alias, or file), it warns and skips the symlink unless you pass `FORCE=1`.

`dev` and `devhub` behave the same (`dev --help` prints `dev`). Keybindings always call the absolute `devhub` binary, never `dev`.

Ensure the install directory is on your `PATH`, then:

```bash
devhub          # or: dev
devhub shell install
devhub doctor
```

`devhub` starts the daemon if needed, starts the isolated tmux server, creates/attaches session `dh-main`, writes `~/.devhub/tmux.conf`, and applies keybindings. It never edits `~/.tmux.conf`.

`devhub shell install` writes `~/.devhub/shell/devhub.zsh` and appends one guarded `source` line to `~/.zshrc` (idempotent, backs up `.zshrc` first). The hook only adds a project badge when `DEVHUB_PROJECT` is set, so ordinary terminals are unchanged.

## Daemon and local API

`devhub daemon` listens forever on a unix socket (`chmod 600`). TCP is off by default and, if enabled, binds only `127.0.0.1` with `Authorization: Bearer` from `~/.devhub/token`.

```bash
devhub daemon status
devhub daemon stop
devhub daemon restart
devhub daemon logs
devhub daemon install     # ~/Library/LaunchAgents/dev.devhub.daemon.plist
devhub daemon uninstall
```

Unix-socket example:

```bash
curl --unix-socket ~/.devhub/devhub.sock http://devhub/v1/health
curl --unix-socket ~/.devhub/devhub.sock http://devhub/v1/sessions
curl --unix-socket ~/.devhub/devhub.sock \
  -H 'Content-Type: application/json' \
  -d '{"project":"krayon","kind":"shadow","command":"npm run dev"}' \
  http://devhub/v1/sessions
```

Logs: `~/.devhub/logs/daemon.log` (rotated ~5MB × 3). Pidfile: `~/.devhub/daemon.pid`.

## MCP (Cursor)

```bash
devhub mcp install --cursor   # backup + merge ~/.cursor/mcp.json; other servers kept
```

That adds `{ "mcpServers": { "devhub": { "command": "<abs devhub>", "args": ["mcp"] } } }`. `devhub mcp` is a stdio server that proxies through the local API. Mutations follow `mcp.allow_mutations` (default true). `free_port` needs `mcp.allow_port_kill` (default false). `read_logs` scrubs secrets (AWS keys, PEM, Bearer, `*_SECRET=`, `*_TOKEN=`, GCP service accounts, plus exact values from the secrets broker).

GCP MCP tools: `gcp_list_projects`, `gcp_logs`, `gcp_bq_query`, `gcp_run_status`, `gcp_pubsub_peek`, `gcp_gcs_list`. Prod aliases with `require_confirm` are refused over MCP unless `gcp.allow_prod_via_mcp` is true. E2E: `e2e_run`, `e2e_status`, `e2e_failures`, `e2e_logs`. `secrets_list` returns names only. `events_recent` lists crashes and failures. Project scan: `scan_projects` (read-only), `add_project` / `remove_project` (mutations).

## Skills and rules

Source of truth: `~/.devhub/skills` (git repo). Layout:

```
~/.devhub/skills/
  skills/<name>/SKILL.md
  rules/<name>.md
  .sync-manifest.json     # only these files may be deleted on unsync
```

```bash
devhub skills new go-style
devhub skills ls
devhub skills get go-style
devhub skills sync
devhub skills unsync
devhub skills doctor
devhub skills cp go-style
devhub rules new go
```

Cursor is enabled by default (`skills.targets: [cursor]`). Generated files start with a managed-by-devhub header. Edit in `~/.devhub/skills`, not the copies.

## Project profiles (`devhub.yaml`)

```yaml
name: krayon
env:
  NODE_ENV: development
env_file: .env
windows:
  - name: v1
    cwd: .
shadows:
  - name: web
    command: npm run dev
    port: 3000
    ready:
      http: http://127.0.0.1:3000
    restart: on-failure   # no | on-failure | always
    depends_on: []
skills:
  - go-style
```

```bash
devhub init                 # detect package.json / go.mod / docker-compose / Makefile
devhub profile validate
devhub up [project]         # -y frees conflicting ports
devhub down [project]
```

The daemon restarts `on-failure` / `always` shadows with exponential backoff (max 5). Restart count shows in the shadows TUI.

## GCP gateway

Auth is Application Default Credentials only (`gcloud auth application-default login`). Devhub never stores service account keys. Optional impersonation uses `impersonate_service_account`.

```yaml
# ~/.devhub/config.yaml
gcp:
  allow_prod_via_mcp: false
  impersonate_service_account: ""   # optional
  rate_limit_per_min: 30
  projects:
    - id: my-proj-dev
      alias: dev
      allow: [logs.read, bq.query, pubsub.peek, run.status, secrets.read]
      bq: {max_bytes_billed: 1GB, dry_run_first: true}
    - id: my-proj-prod
      alias: prod
      allow: [logs.read, run.status]
      require_confirm: true
```

A per-project `gcp.projects` block in `devhub.yaml` overlays by alias. CLI: `devhub gcp ls`, `devhub gcp logs dev --filter 'severity>=ERROR' --since 1h`, `devhub gcp bq dev "SELECT 1"`, `devhub gcp run dev`, `devhub gcp pubsub peek dev mysub`, `devhub gcp auth check`. Prefix `g` opens the TUI.

## E2E sandbox

Runs Playwright (or any command) through the Docker Engine API (OrbStack, Docker Desktop, or Colima). The repo is copied in as a tar of tracked files plus the current diff — never bind-mounted read-write. Sidecars share a per-run network. Teardown always runs.

```yaml
# devhub.yaml
e2e:
  image: devhub/krayon-e2e
  dockerfile: ./e2e/Dockerfile
  cache_volumes: [node_modules, ~/.cache/ms-playwright]
  command: npx playwright test
  services:
    - name: db
      image: postgres:16
      env: {POSTGRES_PASSWORD: devhub}
      ready: {tcp: 5432}
  env: {BASE_URL: http://app:3000}
  secrets: [STRIPE_TEST_KEY]
  artifacts: [playwright-report, test-results]
  timeout: 20m
  resources: {cpus: 2, memory: 4g}
```

CLI: `devhub e2e build|run|ls|logs|open|cancel|prune`. Prefix `e` in tmux.

## Secrets broker

References in `env` and `e2e.secrets`: `keychain://name`, `gcp://alias/secret[#version]`, `op://Vault/Item/field` (if `op` is installed). Values are resolved in the daemon and injected only as process/container env. They are never written to disk, logs, the registry, the audit table (names only), tmux scrollback, or API/MCP responses. The log scrubber also redacts exact cached values. `devhub secrets lock` clears the 15-minute memory cache.

```bash
devhub secrets set stripe-test   # hidden prompt, Keychain service "devhub"
devhub secrets ls                # names and sources
devhub secrets check [project]
devhub secrets rm stripe-test
```

There is no MCP tool that returns a secret value.

## Alerts

```yaml
alerts:
  quiet_hours: { start: "23:00", end: "08:00", location: Asia/Kolkata }
  slack: { webhook: "" }    # empty = disabled
  rules:
    - on: shadow.crashed
      notify: [macos]
    - on: resource.threshold
      when: { rss_mb: 2048, cpu_pct: 200, for: 30s }
      notify: [macos]
    - on: e2e.failed
      notify: [macos]
      actions: [open_report]
```

Events: `shadow.crashed`, `shadow.restarted_max`, `e2e.finished`, `e2e.failed`, `resource.threshold`, `port.conflict`, `gcp.error`. Deduped for 60s. Clicking a `terminal-notifier` alert focuses the tmux window or opens the E2E report. CLI: `devhub alerts ls|test|mute 1h`. Prefix `a`.

## Janitor

```bash
devhub ports
devhub free 3000            # confirm unless -y
devhub orphans
devhub orphans --kill
devhub janitor              # TUI: Ports / Orphans tabs
```

Kill denylist: pid &lt; 500, names `launchd` / `WindowServer`, other users' processes.

## Concepts

| Thing | tmux | Notes |
| --- | --- | --- |
| Project `krayon` | session `dh-krayon` | cwd is the project path (`-c`); missing dirs are an error, never `$HOME` |
| Active terminal | window `v1`, `v2`, … | shown to you; names are per-project and renamable |
| Shadow terminal | window in `dh-shadow-krayon` | long-running (`npm run dev`); same project cwd; survives leaving the UI |
| Registry | `~/.devhub/devhub.db` | reconciled against `tmux list-panes` on every refresh |

If a name exists in more than one project, pass `project/name` (`krayon/v1`).

Every pane gets `DEVHUB_PROJECT` and `DEVHUB_COLOR`. The prompt shows a coloured `[project]` badge; the tmux status-left and window titles use the same colour.

## Examples

```bash
devhub project add krayon ~/code/krayon
devhub project ls

devhub scan projects              # ~/Desktop, then picker
devhub scan                       # same
devhub scan projects --all        # home (add --include-volumes for /Volumes)
devhub scan projects --no-ui
devhub scan projects --json --refresh

devhub new --project krayon          # starts in the krayon directory
devhub new --project krayon --name v3

devhub shadow krayon -- npm run dev  # also in the krayon directory
devhub ls
devhub ls --all
devhub peek npm-run-dev
devhub attach v1
devhub restart npm-run-dev
devhub kill v2
```

Popups:

```bash
devhub picker      # active terminals
devhub shadows     # background processes
devhub top         # CPU / RAM
devhub projects    # switch project session
devhub scan-ui     # project scanner picker (cache)
devhub skills-ui   # skills / rules
devhub janitor     # ports / orphans
devhub updown      # profile up / down
devhub gcp-ui      # GCP gateway
devhub e2e-ui      # E2E runs
devhub alerts-ui   # recent alerts
```

## Keybindings

Default `keys.mode` is `both`: **Option+key** (no prefix) and **Ctrl-Space** then a key. Generated into `~/.devhub/tmux.conf`.

macOS never sends a bare Option press, so Option+key requires the terminal to treat Option as Meta/Esc+. `devhub doctor` prints the steps for your `TERM_PROGRAM`.

| Keys | Action |
| --- | --- |
| `M-s` / `prefix s` | Fuzzy list of **active** terminals. Enter switches, `n` new, `r` rename, `x` kill (confirm). |
| `M-h` / `prefix h` | **Shadow** list. Enter peek (live tail), `R` restart, `x` kill, `a` add, `o` promote. Restart count is shown. |
| `M-r` / `prefix r` | Live **top** table (2s). `c` sort CPU, `m` sort memory. |
| `M-p` / `prefix p` | Project switcher (`x` remove, `e` rename, `+` scan) |
| `M-P` / `prefix P` | Scan picker (checkbox add/remove; `r` Desktop, `R` --all) |
| `M-k` / `prefix k` | Skills / rules (`c` copy, `e` edit, `s` sync, `d` doctor) |
| `M-o` / `prefix o` | Janitor (tab Ports/Orphans, `x` kill, `a` adopt) |
| `M-u` / `prefix u` | Profile up / down |
| `M-g` / `prefix g` | GCP gateway |
| `M-e` / `prefix e` | E2E runs (Enter logs, `r` rerun, `o` report) |
| `M-a` / `prefix a` | Recent alerts |
| `M-1`…`M-9` / `prefix 1`…`9` | Jump to `v1`…`v9` of the current project |
| Esc / `q` | Close any popup |

`M-b`, `M-f`, and `M-d` are **not** bound (readline word motion / kill).

### Option as Meta

- **iTerm2:** Profiles > Keys > Left Option key = Esc+. Or `devhub setup iterm` (writes a Dynamic Profile).
- **Terminal.app:** Settings > Profiles > Keyboard > Use Option as Meta key. Or `devhub setup terminal` (asks first).

This disables typing special characters such as © / π with Option in that profile.

Status bar: left is the project badge; right is `<n> active | <n> shadow | <cpu%> | <ram>` via cached `~/.devhub/status.json`.

## Config

`~/.devhub/config.yaml`:

```yaml
prefix: C-Space
keys:
  mode: both          # option | prefix | both
projects:
  krayon: /Users/you/code/krayon
project_colors:
  krayon: "#89B4FA"   # optional; otherwise hashed from the name
default_shadow_commands:
  krayon:
    - npm run dev
api:
  enable_tcp: false
  tcp_port: 7420
mcp:
  allow_mutations: true
  allow_port_kill: false
  allow_prod_via_mcp: false
skills:
  targets: [cursor]
janitor:
  process_names: [node, npm, pnpm, bun, go, python, vite, next]
gcp:
  allow_prod_via_mcp: false
  rate_limit_per_min: 30
  projects: []
e2e:
  concurrency: 2
  retention: 7d
scan:
  roots: []
  exclude: []
  depth_desktop: 6
  depth_all: 10
  include_volumes: false
alerts:
  quiet_hours: { start: "23:00", end: "08:00", location: Asia/Kolkata }
  rules: []
```

If Starship is detected, `devhub shell install` prints an `env_var.DEVHUB_PROJECT` module snippet instead of editing `PROMPT`.

Data files: `~/.devhub/devhub.db`, `~/.devhub/status.json`, `~/.devhub/tmux.conf`, `~/.devhub/shell/devhub.zsh`, `~/.devhub/devhub.sock`, `~/.devhub/daemon.pid`, `~/.devhub/token`, `~/.devhub/logs/daemon.log`, `~/.devhub/skills/`, `~/.devhub/scan-cache.json`.

## Develop

Tests are CGO-free because process APIs use `ps` and `lsof`, not gopsutil. `make test` still sets `CGO_ENABLED=0` as a belt-and-suspenders. `CGO_ENABLED=1 go test ./...` should also pass.

```bash
make build
make test
make lint
go test ./...
```
