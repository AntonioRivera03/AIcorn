BINARY   := aycorn
APP_DIR  := app
SRV_DIR  := server
UI_DIST  := $(SRV_DIR)/ui/dist

# Embed the current git tag (e.g. v0.1.0) into the binary at build time.
# Falls back to "dev" when git isn't available or there are no tags yet.
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: dev dev-test build build-app build-app-dev build-md-convert build-mcp build-server typecheck test test-app test-server install upgrade stop clean backup restore backup-test restore-test sync-agents check-agents

# Bundled Markdown is the single source for runtime and repository agents.
sync-agents:
	cd $(SRV_DIR) && go run ./cmd/sync-agents

check-agents:
	cd $(SRV_DIR) && go run ./cmd/sync-agents -check

# Pick up the local cluster created by scripts/k8s-local.sh without changing
# the user's global Kubernetes config. An explicit KUBECONFIG (even empty) wins.
define load-dev-kubeconfig
if [ "$${KUBECONFIG+x}" != x ]; then \
	aycorn_kubeconfig="$${XDG_CONFIG_HOME:-$$HOME/.config}/aycorn/kubeconfig-aycorn"; \
	if [ -f "$$aycorn_kubeconfig" ]; then export KUBECONFIG="$$aycorn_kubeconfig"; fi; \
fi
endef

# Development: build frontend with dev icon, then start Go server against your
# personal DB (no AYCORN_DB override → internal/appdb.ResolveDBPath() falls
# back to <UserConfigDir>/aycorn/app.db, same DB the installed binary uses).
dev: build-app-dev build-mcp
	@$(load-dev-kubeconfig); \
    trap 'kill 0' INT; \
    cd $(SRV_DIR) && go run ./cmd/web; \
    wait

# Development against a disposable test DB: pins AYCORN_DB to server/app.db so
# it never touches your personal data. Safe to `rm -f server/app.db` anytime.
dev-test: build-app-dev build-mcp
	@$(load-dev-kubeconfig); \
    trap 'kill 0' INT; \
    cd $(SRV_DIR) && AYCORN_DB=./app.db go run ./cmd/web; \
    wait

# Full release build: React → embed → single Go binary
build: build-app build-mcp build-server

build-app:
	cd $(APP_DIR) && npm run build

build-app-dev:
	cd $(APP_DIR) && npx vite build --mode development

# Bundle Plate's markdown serializer into a standalone Node script that the MCP
# server embeds and shells out to. Required before any `go build` that reaches
# server/assets/bin — the embed fails loudly without it.
build-md-convert:
	cd $(APP_DIR) && npm run build:md-convert

# The MCP stdio server (Documentation/phase-1-mcp-server.md). Point your MCP
# host at server/bin/aycorn-mcp. Needs `node` on PATH at runtime.
build-mcp: build-md-convert
	cd $(SRV_DIR) && CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/aycorn-mcp ./cmd/mcp
	@echo "Binary ready: $(SRV_DIR)/bin/aycorn-mcp"

# Run TypeScript type check without building
typecheck:
	cd $(APP_DIR) && npx tsc -b --noEmit

test: test-server test-app

# Go tests. Depends on the markdown bundle for the same reason `build-mcp` does
# — internal/markdown embeds it, so the package won't compile without it. Its
# tests exercise the real bundle under node, and skip if node is missing.
test-server: build-md-convert
	cd $(SRV_DIR) && go test ./...

# Vitest (vitest.config.ts). Headless, no browser or DOM needed.
test-app:
	cd $(APP_DIR) && npm test

build-server: build-md-convert
	cd $(SRV_DIR) && CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(VERSION)" -o ../$(BINARY) ./cmd/web
	@echo "Binary ready: ./$(BINARY) ($(VERSION))"

# Install the binary system-wide so `aycorn` works from anywhere.
# macOS/Linux: copies to /usr/local/bin (may require sudo).
# To uninstall: sudo rm /usr/local/bin/aycorn
install: build build-mcp
	sudo cp $(BINARY) /usr/local/bin/$(BINARY)
	sudo cp $(SRV_DIR)/bin/aycorn-mcp /usr/local/bin/aycorn-mcp
	@echo "Installed: $$(which aycorn)"

# Gracefully stop the running aycorn process (no-op if it isn't running).
# The server handles SIGTERM cleanly — in-flight requests finish before it exits.
stop:
	-pkill -TERM -x aycorn
	@echo "Stopped aycorn (if it was running). Run 'aycorn' to start again."

# Rebuild, reinstall, and stop the old process. Run after `git pull`.
# Run 'aycorn' afterwards to start the new version. The next start automatically
# snapshots the DB before applying any schema migration, so upgrades can't lose data.
upgrade:
	$(MAKE) stop
	$(MAKE) install
	@echo "Upgraded to $$(aycorn --version)"

# Snapshot / restore your personal database via the binary's subcommands (no
# AYCORN_DB override → same DB `make dev` and the installed binary use).
backup:
	cd $(SRV_DIR) && go run ./cmd/web backup $(DEST)

restore:
	cd $(SRV_DIR) && go run ./cmd/web restore $(SRC)

# Snapshot / restore the disposable TEST database (server/app.db) — pairs with
# `make dev-test`.
backup-test:
	cd $(SRV_DIR) && AYCORN_DB=./app.db go run ./cmd/web backup $(DEST)

restore-test:
	cd $(SRV_DIR) && AYCORN_DB=./app.db go run ./cmd/web restore $(SRC)

clean:
	rm -f $(BINARY)
	rm -rf $(UI_DIST) dist-release
