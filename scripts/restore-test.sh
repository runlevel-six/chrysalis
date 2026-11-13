#!/usr/bin/env bash
# Copyright 2026 Jason Hall
# SPDX-License-Identifier: Apache-2.0
#
# The tested restore from docs/DESIGN.md §7.4, run against the real binary:
#
#   1. start a server in dev mode and add a board, columns, cards and an
#      attachment through the HTTP API;
#   2. back it up with `chrysalis admin backup` while it's running;
#   3. restore the backup into a fresh data directory with
#      `chrysalis admin restore`;
#   4. start a server on the restored data and check the board and the
#      attachment's bytes are the same.
#
# Needs go, curl and jq. Everything happens in a temporary directory.
#
# Usage: scripts/restore-test.sh  (PORT=18080 by default)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
port="${PORT:-18080}"
base="http://localhost:${port}"
work="$(mktemp -d)"
server_pid=""

cleanup() {
  if [[ -n "$server_pid" ]]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -rf "$work"
}
trap cleanup EXIT

fail() {
  echo "restore-test: FAIL: $*" >&2
  if [[ -f "$work/server.log" ]]; then
    echo "--- server log ---" >&2
    tail -n 40 "$work/server.log" >&2
  fi
  exit 1
}

echo "restore-test: building"
(cd "$root" && go build -o "$work/chrysalis" ./cmd/chrysalis)

# chrysalis runs the binary with dev-mode settings for the data directory $1.
chrysalis() {
  local data="$1"
  shift
  env CHRYSALIS_AUTH_MODE=dev CHRYSALIS_DEV_MODE=true \
    CHRYSALIS_BASE_URL="$base" CHRYSALIS_LISTEN_ADDR="127.0.0.1:${port}" \
    CHRYSALIS_DATA_DIR="$data" CHRYSALIS_BACKUP_DIR="$work/backups" \
    CHRYSALIS_LOG_LEVEL=warn \
    "$work/chrysalis" "$@"
}

start_server() {
  chrysalis "$1" serve >>"$work/server.log" 2>&1 &
  server_pid=$!
  for _ in $(seq 1 100); do
    if curl -fsS "$base/readyz" >/dev/null 2>&1; then
      return 0
    fi
    kill -0 "$server_pid" 2>/dev/null || fail "server exited during startup"
    sleep 0.1
  done
  fail "server didn't become ready"
}

stop_server() {
  kill "$server_pid"
  wait "$server_pid" 2>/dev/null || true
  server_pid=""
}

api() {
  local method="$1" path="$2" body="${3:-}"
  if [[ -n "$body" ]]; then
    curl -fsS -X "$method" -H 'Content-Type: application/json' --data "$body" "$base$path"
  else
    curl -fsS -X "$method" "$base$path"
  fi
}

# snapshot prints the parts of a board that must survive a restore, in a
# stable order.
snapshot() {
  api GET "/api/v1/boards/$1/detail" | jq -S '{
    board: .board.name,
    columns: [.columns[] | {id, name}],
    cards: [.cards[] | {id, columnId, title}] | sort_by(.id)
  }'
}

echo "restore-test: creating data"
start_server "$work/a"
board="$(api POST /api/v1/boards '{"name":"Restore test"}' | jq -r .id)"
todo="$(api POST "/api/v1/boards/$board/columns" '{"name":"To do"}' | jq -r .id)"
done_col="$(api POST "/api/v1/boards/$board/columns" '{"name":"Done"}' | jq -r .id)"
for title in "Water the plants" "Call bob" "Book the boiler service"; do
  api POST "/api/v1/columns/$todo/cards" "$(jq -nc --arg t "$title" '{title: $t}')" >/dev/null
done
card="$(api POST "/api/v1/columns/$done_col/cards" '{"title":"Pay the bills"}' | jq -r .id)"

head -c 200000 /dev/urandom >"$work/upload.bin"
attachment="$(curl -fsS -F "file=@$work/upload.bin;filename=notes.bin" \
  "$base/api/v1/cards/$card/attachments" | jq -r .id)"
[[ -n "$attachment" && "$attachment" != null ]] || fail "upload failed"

snapshot "$board" >"$work/before.json"
[[ "$(jq '.cards | length' "$work/before.json")" == 4 ]] || fail "expected 4 cards before backup"

echo "restore-test: backing up while the server runs"
chrysalis "$work/a" admin backup || fail "backup failed"
stop_server

echo "restore-test: restoring into a fresh data directory"
chrysalis "$work/b" admin restore latest || fail "restore failed"
# A second restore over the restored data must be refused without --force.
if chrysalis "$work/b" admin restore latest 2>/dev/null; then
  fail "restore over existing data succeeded without --force"
fi

echo "restore-test: checking the restored instance"
start_server "$work/b"
snapshot "$board" >"$work/after.json"
diff -u "$work/before.json" "$work/after.json" || fail "board differs after restore"
curl -fsS "$base/api/v1/attachments/$attachment" -o "$work/download.bin" || fail "attachment missing after restore"
cmp "$work/upload.bin" "$work/download.bin" || fail "attachment bytes differ after restore"
stop_server

echo "restore-test: OK"
