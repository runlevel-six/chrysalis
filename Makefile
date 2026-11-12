# Copyright 2026 Jason Hall
# SPDX-License-Identifier: Apache-2.0

.PHONY: hooks
hooks: ## Install git hooks (denylist check + betterleaks)
	git config core.hooksPath scripts/hooks
	@echo "hooks installed from scripts/hooks"
