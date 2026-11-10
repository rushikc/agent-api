PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin
FORCE ?=

.PHONY: build install test lint

# Pure Go (modernc sqlite). Avoids Darwin CGO link issues with gopsutil extras.
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
