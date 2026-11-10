PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin
FORCE ?=

.PHONY: build install test lint dev fixed frontend backend e2e typecheck

# Args for bin/devhub in `make dev` / `make fixed` (see COMMANDS.md).
DEVHUB_ARGS ?= daemon

# Belt-and-suspenders: process APIs are ps/lsof (no CGO). Keep CGO off in make.
export CGO_ENABLED := 0

build:
	go build -o bin/devhub ./cmd/devhub

install: build
	@set -e; \
	if mkdir -p "$(BINDIR)" 2>/dev/null && [ -w "$(BINDIR)" ]; then dest="$(BINDIR)"; \
	elif mkdir -p "$(HOME)/bin" && [ -w "$(HOME)/bin" ]; then dest="$(HOME)/bin"; echo "PREFIX $(BINDIR) not writable; using $$dest"; \
	else echo "could not write $(BINDIR) or $(HOME)/bin; binary is at ./bin/devhub"; exit 1; fi; \
	install -m 755 bin/devhub "$$dest/devhub"; \
	echo "installed $$dest/devhub"; \
	devlink="$$dest/dev"; \
	if [ -L "$$devlink" ]; then \
		target=$$(readlink "$$devlink"); \
		if [ "$$target" = "devhub" ] || [ "$$target" = "$$dest/devhub" ]; then \
			echo "symlink $$devlink -> $$target"; \
			exit 0; \
		fi; \
		if [ -z "$(FORCE)" ]; then \
			echo "warning: $$devlink is a symlink to $$target; skip (FORCE=1 to overwrite)"; \
			exit 0; \
		fi; \
	elif [ -e "$$devlink" ]; then \
		if [ -z "$(FORCE)" ]; then \
			echo "warning: $$devlink exists; skip (FORCE=1 to overwrite)"; \
			ls -l "$$devlink"; \
			exit 0; \
		fi; \
	fi; \
	found=$$(command -v dev 2>/dev/null || true); \
	if [ -n "$$found" ] && [ "$$found" != "$$devlink" ] && [ -z "$(FORCE)" ]; then \
		echo "warning: 'dev' already on PATH at $$found; skip symlink (FORCE=1 to create $$devlink anyway)"; \
		ls -l "$$found" 2>/dev/null || true; \
		exit 0; \
	fi; \
	ln -sfn devhub "$$devlink"; \
	echo "linked $$devlink -> devhub"

test:
	go test ./...

lint:
	go vet ./...

typecheck:
	go build ./...

# Common commands (COMMANDS.md). dev/backend rebuild and restart on change; fixed builds once.
dev:
	@./scripts/dev.sh $(DEVHUB_ARGS)

backend: dev

fixed: build
	./bin/devhub $(DEVHUB_ARGS)

frontend:
	@echo "agent-api has no frontend (Go CLI + daemon); not applicable."

e2e:
	@echo "agent-api has no e2e suite; nothing to run."
