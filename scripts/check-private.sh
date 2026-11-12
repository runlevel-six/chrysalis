#!/usr/bin/env bash
# Copyright 2026 Jason Hall
# SPDX-License-Identifier: Apache-2.0
#
# Fails if any term from local/denylist.txt (case-insensitive) appears in the
# staged diff or in a commit message. The denylist is private and gitignored,
# so this check is a no-op where it doesn't exist (e.g. CI).
#
# Usage:
#   scripts/check-private.sh            # check added lines in the staged diff
#   scripts/check-private.sh --msg FILE # check a commit message file
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
denylist="$root/local/denylist.txt"
[[ -f "$denylist" ]] || exit 0

# Strip comments, blank lines and CRs from the denylist.
terms="$(sed -e 's/\r$//' -e '/^[[:space:]]*#/d' -e '/^[[:space:]]*$/d' "$denylist")"
[[ -n "$terms" ]] || exit 0

if [[ "${1:-}" == "--msg" ]]; then
  what="commit message"
  content="$(grep -v '^#' "${2:?missing message file}" || true)"
else
  what="staged changes"
  # Added lines only, plus file paths, so new file names are checked too.
  content="$(git diff --cached --no-color -U0 | grep -E '^(\+|diff --git)' | grep -v '^+++' || true)"
fi

hits="$(printf '%s\n' "$content" | grep -n -i -F -f <(printf '%s\n' "$terms") || true)"
if [[ -n "$hits" ]]; then
  echo "check-private: denylisted term found in $what:" >&2
  printf '%s\n' "$hits" | head -20 >&2
  echo "Remove it, or see docs/DESIGN.md §0.1." >&2
  exit 1
fi
