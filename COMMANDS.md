# Common commands

Every project in `~/Desktop/Projects` (agent-api, doc-mate, krayon.ai, my-portfolio, pennywise) uses the same command names. This section is identical in every repo. The "This project" section below says what each command runs here.

| Command | What it does |
|---|---|
| `dev` | Frontend and backend together, with hot reload. Frontend only when the project has no backend. |
| `fixed` | Frontend and backend together, with no hot reload or file watchers. Uses a production build where that makes sense. |
| `frontend` | Frontend only, with hot reload. |
| `backend` | Backend only, with hot reload. |
| `test` | Unit and integration tests. Never runs e2e. |
| `e2e` | The full end-to-end suite. |
| `build` | Production build. |
| `lint` | Linters. |
| `typecheck` | Type checking with no output files. |

## Rules

- Run them as `npm run <command>`. If the repo has `pnpm-lock.yaml`, use `pnpm <command>` instead. Go projects use `make <command>` with the same names.
- Use `dev` while editing. Use `fixed` when you need a stable stack with nothing restarting, such as when testing a flow end to end by hand or recording.
- `test` never runs e2e. Run `e2e` only when the user explicitly asks for it.
- A command that doesn't apply to a project prints a one-line message and exits 0. It doesn't fail.
- Older script names still work as aliases. Project-specific scripts (deploy, migrations, tooling) stay as they are and are listed in each project's own docs.

## This project: agent-api (Go, make)

| Command | Runs |
|---|---|
| `make dev` | `scripts/dev.sh`: builds `bin/devhub`, runs `./bin/devhub daemon`, and rebuilds and restarts it when `cmd/`, `internal/`, `go.mod` or `go.sum` change. Uses `watchexec` if installed, otherwise polls every 2s. |
| `make fixed` | `make build`, then `./bin/devhub daemon` (no watcher) |
| `make frontend` | Prints "not applicable" (no frontend) |
| `make backend` | Same as `make dev` |
| `make test` | `go test ./...` |
| `make e2e` | Prints "no e2e suite" and exits 0 |
| `make build` / `lint` / `typecheck` | `go build -o bin/devhub ./cmd/devhub` / `go vet ./...` / `go build ./...` |

Notes:
- Override the daemon args with `make dev DEVHUB_ARGS="..."`, for example `DEVHUB_ARGS=monitor`.
- `make dev` and `make fixed` run a second daemon from `./bin`. Stop the installed one first (`devhub daemon stop`), because both use the same pid file and socket in your config dir.
- `make install` is unchanged.
