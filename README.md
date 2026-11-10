# devhub

A single-binary terminal hub for macOS. It owns an isolated tmux server (`-L devhub`) and a Bubble Tea UI so every project terminal and background process lives in one place.

v1 covers session management, keybindings, and resource monitoring. A daemon/HTTP API can wrap `internal/core.Service` later without rewriting this layer.

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

`devhub` starts the isolated tmux server if needed, creates/attaches session `dh-main`, writes `~/.devhub/tmux.conf`, and applies keybindings. It never edits `~/.tmux.conf`.

`devhub shell install` writes `~/.devhub/shell/devhub.zsh` and appends one guarded `source` line to `~/.zshrc` (idempotent, backs up `.zshrc` first). The hook only adds a project badge when `DEVHUB_PROJECT` is set, so ordinary terminals are unchanged.

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
devhub projects    # switch project session (creates dh-<project> in the project path if needed)
```

## Keybindings

Default `keys.mode` is `both`: **Option+key** (no prefix) and **Ctrl-Space** then a key. Generated into `~/.devhub/tmux.conf`.

macOS never sends a bare Option press, so Option+key requires the terminal to treat Option as Meta/Esc+. `devhub doctor` prints the steps for your `TERM_PROGRAM`.

| Keys | Action |
| --- | --- |
| `M-s` / `prefix s` | Fuzzy list of **active** terminals. Enter switches, `n` new, `r` rename, `x` kill (confirm). |
| `M-h` / `prefix h` | **Shadow** list. Enter peek (live tail), `R` restart, `x` kill, `a` add, `o` promote. |
| `M-r` / `prefix r` | Live **top** table (2s). `c` sort CPU, `m` sort memory. |
| `M-p` / `prefix p` | Project switcher |
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
```

If Starship is detected, `devhub shell install` prints an `env_var.DEVHUB_PROJECT` module snippet instead of editing `PROMPT`.

Data files: `~/.devhub/devhub.db`, `~/.devhub/status.json`, `~/.devhub/tmux.conf`, `~/.devhub/shell/devhub.zsh`.

## Develop

```bash
make build
make test
make lint
```
