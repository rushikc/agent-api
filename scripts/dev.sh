#!/usr/bin/env bash
# make dev: build bin/devhub, run it, and rebuild + restart when Go sources change.
# Uses watchexec when installed; otherwise polls cmd/, internal/, go.mod and go.sum every 2 seconds.
# Args are passed to devhub (default: daemon).
set -u
cd "$(dirname "$0")/.."
export CGO_ENABLED=0
[ $# -eq 0 ] && set -- daemon

if command -v watchexec >/dev/null 2>&1; then
  exec watchexec -r -e go,mod,sum -- "go build -o bin/devhub ./cmd/devhub && ./bin/devhub $*"
fi

stamp="$(mktemp -t devhub-dev.XXXXXX)"
pid=""

stop() {
  if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
    kill "$pid" 2>/dev/null
    wait "$pid" 2>/dev/null
  fi
  pid=""
}

start() {
  touch "$stamp"
  if go build -o bin/devhub ./cmd/devhub; then
    echo "[dev] running ./bin/devhub $*"
    ./bin/devhub "$@" &
    pid=$!
  else
    echo "[dev] build failed; waiting for changes"
  fi
}

trap 'stop; rm -f "$stamp"; exit 0' INT TERM

echo "[dev] no watcher installed (brew install watchexec for faster reloads); polling every 2s"
start "$@"
while true; do
  sleep 2
  if [ -n "$(find cmd internal go.mod go.sum -newer "$stamp" \( -name '*.go' -o -name go.mod -o -name go.sum \) -print 2>/dev/null | head -n 1)" ]; then
    echo "[dev] change detected; rebuilding"
    stop
    start "$@"
  fi
done
