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

That adds `{ "mcpServers": { "devhub": { "command": "<abs devhub>", "args": ["mcp"] } } }`. `devhub mcp` is a stdio server that proxies through the local API. Mutations follow `mcp.allow_mutations` (default true). `free_port` needs `mcp.allow_port_kill` (default false). `read_logs` scrubs secrets (AWS keys, PEM, Bearer, `*_SECRET=`, `*_TOKEN=`, GCP service accounts).

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
devhub skills-ui   # skills / rules
devhub janitor     # ports / orphans
devhub updown      # profile up / down
```

## Keybindings

Default `keys.mode` is `both`: **Option+key** (no prefix) and **Ctrl-Space** then a key. Generated into `~/.devhub/tmux.conf`.

macOS never sends a bare Option press, so Option+key requires the terminal to treat Option as Meta/Esc+. `devhub doctor` prints the steps for your `TERM_PROGRAM`.

| Keys | Action |
| --- | --- |
| `M-s` / `prefix s` | Fuzzy list of **active** terminals. Enter switches, `n` new, `r` rename, `x` kill (confirm). |
| `M-h` / `prefix h` | **Shadow** list. Enter peek (live tail), `R` restart, `x` kill, `a` add, `o` promote. Restart count is shown. |
| `M-r` / `prefix r` | Live **top** table (2s). `c` sort CPU, `m` sort memory. |
| `M-p` / `prefix p` | Project switcher |
| `M-k` / `prefix k` | Skills / rules (`c` copy, `e` edit, `s` sync, `d` doctor) |
| `M-o` / `prefix o` | Janitor (tab Ports/Orphans, `x` kill, `a` adopt) |
| `M-u` / `prefix u` | Profile up / down |
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
skills:
  targets: [cursor]
janitor:
  process_names: [node, npm, pnpm, bun, go, python, vite, next]
```

If Starship is detected, `devhub shell install` prints an `env_var.DEVHUB_PROJECT` module snippet instead of editing `PROMPT`.

Data files: `~/.devhub/devhub.db`, `~/.devhub/status.json`, `~/.devhub/tmux.conf`, `~/.devhub/shell/devhub.zsh`, `~/.devhub/devhub.sock`, `~/.devhub/daemon.pid`, `~/.devhub/token`, `~/.devhub/logs/daemon.log`, `~/.devhub/skills/`.

## Develop

```bash
make build
make test
make lint
```
