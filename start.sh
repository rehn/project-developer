#!/usr/bin/env bash
# Starts the Go backend (:3001) and Next.js dev server (:3000). Ctrl+C stops both.
set -euo pipefail
cd "$(dirname "$0")"

command -v go >/dev/null   || { echo "Go is required (https://go.dev/dl)"; exit 1; }
command -v node >/dev/null || { echo "Node.js is required"; exit 1; }
command -v git >/dev/null  || { echo "git is required"; exit 1; }
command -v "${CLAUDE_BIN:-claude}" >/dev/null || echo "⚠ claude CLI not found on PATH — the Terminal page won't work (set CLAUDE_BIN)"

[ -d web/node_modules ] || (echo "Installing frontend deps…" && cd web && npm install)

export DATA_DIR="${DATA_DIR:-$PWD/data}"
pids=()
cleanup() { trap - INT TERM EXIT; kill "${pids[@]}" 2>/dev/null || true; wait 2>/dev/null || true; }
trap cleanup INT TERM EXIT

mkdir -p "$DATA_DIR"
(cd backend && go build -o "$DATA_DIR/backend.bin" .)  # build first so Ctrl+C kills the real process
"$DATA_DIR/backend.bin" & pids+=($!)
(cd web && exec ./node_modules/.bin/next dev -p 3000) & pids+=($!)

echo
echo "  UI:  http://localhost:3000"
echo "  API: http://127.0.0.1:3001"
echo
wait -n
