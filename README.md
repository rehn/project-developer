# Project Developer

Web app to manage your projects and drive Claude on them.

- **Backend:** Go (`backend/`) — auth, SQLite, GitHub, git, usage, runs the `claude` CLI
- **Frontend:** Next.js (`web/`) — proxies `/api/*` to the backend

## Run
Quickest: `./start.sh` (starts both, Ctrl+C stops both). Or manually:

Prereqs: Go 1.23+, Node 20+, git, and the [Claude Code CLI](https://claude.com/claude-code) installed and logged in.

    cd backend && go run .          # API on 127.0.0.1:3001
    cd web && npm install && npm run dev   # UI on http://localhost:3000

First visit creates your account. Then: Settings → GitHub token → Projects → add repo → Terminal.

## Config (env, backend)
`ADDR` (127.0.0.1:3001), `DATA_DIR` (./data), `CLAUDE_BIN` (claude), `CLAUDE_CONFIG_DIR`,
`ALLOWED_ORIGINS`, `COOKIE_SECURE=true` (behind HTTPS). Frontend: `BACKEND_URL`.

## Notes
- Usage is read from Claude Code's local logs (`~/.claude/projects`), so it covers activity on this machine only.
- The terminal runs `claude -p` in the project checkout. Modes: plan (read-only), auto-accept edits, bypass permissions.
- Binds to localhost by default — this app can edit files on your machine; put real TLS/auth in front before exposing it.
