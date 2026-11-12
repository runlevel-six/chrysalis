# Copyright 2026 Jason Hall
# SPDX-License-Identifier: Apache-2.0

# Developer tools (sqlc, govulncheck) live in their own module under tools/
# so their dependencies never mix with the app's.
TOOL := go tool -modfile=tools/go.mod

.PHONY: hooks build test generate vuln dev

hooks: ## Install git hooks (denylist check + betterleaks)
	git config core.hooksPath scripts/hooks
	@echo "hooks installed from scripts/hooks"

build: ## Build the server binary into bin/
	go build -o bin/chrysalis ./cmd/chrysalis

test: ## Run all Go tests with the race detector
	go test -race ./...

generate: ## Regenerate sqlc query code after editing SQL
	cd tools && go tool sqlc -f ../sqlc.yaml generate

vuln: ## Check dependencies and the Go toolchain for known vulnerabilities
	$(TOOL) govulncheck ./...

dev: ## Run locally with the fixed dev user (data in ./data)
	CHRYSALIS_BASE_URL=http://localhost:8080 CHRYSALIS_DATA_DIR=./data \
	CHRYSALIS_AUTH_MODE=dev CHRYSALIS_DEV_MODE=true go run ./cmd/chrysalis
