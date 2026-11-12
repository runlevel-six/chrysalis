# Copyright 2026 Jason Hall
# SPDX-License-Identifier: Apache-2.0

# Developer tools (sqlc, govulncheck) live in their own module under tools/
# so their dependencies never mix with the app's.
TOOL := go tool -modfile=tools/go.mod

.PHONY: hooks web build test generate vuln dev web-dev

hooks: ## Install git hooks (denylist check + betterleaks)
	git config core.hooksPath scripts/hooks
	@echo "hooks installed from scripts/hooks"

web: ## Install frontend deps and build the SPA into web/dist
	cd web && npm ci && npm run build

build: web ## Build the server binary (with the SPA embedded) into bin/
	go build -o bin/chrysalis ./cmd/chrysalis

test: ## Run all Go tests with the race detector
	go test -race ./...

generate: ## Regenerate sqlc query code after editing SQL
	cd tools && go tool sqlc -f ../sqlc.yaml generate

vuln: ## Check dependencies and the Go toolchain for known vulnerabilities
	$(TOOL) govulncheck ./...

dev: ## Run the server locally as the fixed dev user (data in ./data) on :8080
	CHRYSALIS_BASE_URL=http://localhost:8080 CHRYSALIS_DATA_DIR=./data \
	CHRYSALIS_AUTH_MODE=dev CHRYSALIS_DEV_MODE=true go run ./cmd/chrysalis

web-dev: ## Run the Vite dev server on :5173 with hot reload (use alongside make dev)
	cd web && npm run dev
