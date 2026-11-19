# Project Developer

Web app to manage your projects and drive Claude on them.

- **Backend:** Go (`backend/`) — auth, SQLite, GitHub, git, usage, runs the `claude` CLI
- **Frontend:** Next.js (`web/`) — proxies `/api/*` to the backend

## Run
Quickest: `./start.sh` (starts both, Ctrl+C stops both). Or manually:

Prereqs: Go 1.26+, Node 20+, git, and the [Claude Code CLI](https://claude.com/claude-code) installed and logged in.

    cd backend && go run .          # API on 127.0.0.1:3001
    cd web && npm install && npm run dev   # UI on http://localhost:3000

First visit creates your account. Then: Settings (top right) → GitHub token → + in the project tab bar → add repo → pick its tab.

## Config (env, backend)
`ADDR` (127.0.0.1:3001), `DATA_DIR` (./data), `CLAUDE_BIN` (claude), `CLAUDE_CONFIG_DIR`,
`GITHUB_API_URL` (GitHub Enterprise), `ALLOWED_ORIGINS`, `COOKIE_SECURE=true` (behind HTTPS), `TRUST_PROXY=true` (behind a gateway/reverse proxy). Frontend: `BACKEND_URL`.

## Notes
- Header gauges: plan limits (5-hour / weekly %) come live from Anthropic via your Claude Code login (undocumented endpoint, read-only, token never leaves the backend).
- Footer: pick the model new Claude sessions in the selected project start with (stored per project) (Default, Opus, Sonnet, Haiku, Fable, or any alias/model ID under "Other…"); it is passed as `claude --model`. Restart a running session to switch.
- The Terminal page is a real interactive Claude Code session (PTY + xterm.js) in the project checkout, with Claude's own permission prompts and slash commands. Sessions keep running when you leave the page or reload, and reattach with recent output replayed. If you are signed out it offers "Sign in with Claude" (runs `claude auth login`). The CLI is found via `CLAUDE_BIN`, `PATH`, common install paths, or the copy bundled with the VS Code extension.
- Settings → Kubernetes access: upload or paste a kubeconfig and `kubectl` works in every Claude session (`$KUBECONFIG` points at a file on the data volume). It works for any reachable cluster, including the one the app itself runs in; restart running sessions to pick it up.
- Projects can ask each other questions: every Claude session gets an `ask_project` tool (and `list_projects`) that runs a one-shot, read-only Claude in the other project's checkout and returns its answer, e.g. the API consumer asking the API builder. Each session may ask `ASK_LIMIT` questions (default 10; restarting the session resets it).
- Binds to localhost by default — this app can edit files on your machine; put real TLS/auth in front before exposing it.

## Docker
The image is Debian + the Go server (serving the static UI) + `git` + GitHub CLI (`gh`) + Go + Node.js + Python + Helm + kubectl + Claude Code, one process on port 8080, running as non-root.

    docker build -t project-developer .                                  # add --platform linux/amd64 for most clusters
    docker run -p 8080:8080 -v pd-data:/data project-developer           # http://localhost:8080

Mount a volume at `/data`: it holds the database, repo checkouts, and `HOME` (`/data/home`), so Claude's login and history survive restarts. Claude Code is pinned with `--build-arg CLAUDE_CODE_VERSION=...` (auto-update is off; rebuild to upgrade).

## Kubernetes (Helm, via OCI)
The chart lives in [charts/project-developer](charts/project-developer).

### Releasing
Push a version tag and the [release workflow](.github/workflows/release.yml) does the rest:

    git tag v0.1.0 && git push origin v0.1.0

1. Builds the multi-arch (amd64 + arm64) image and pushes it to **Docker Hub** as `<user>/project-developer:0.1.0` (and `0.1`, plus `latest` for non-prerelease tags).
2. Packages the Helm chart (defaulting to that image) and pushes it to GHCR as an OCI chart.
3. Creates a **GitHub Release** with generated notes and the chart `.tgz` attached. Tags with a suffix (`v0.2.0-rc.1`) become pre-releases.

One-time setup: add repository secrets `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` (a Docker Hub access token with Read & Write scope). Tags must look like `v1.2.3`.

### Installing
    # Helm >= 3.8 (older versions: export HELM_EXPERIMENTAL_OCI=1). The chart package on GHCR is private by default (make it public, or log in first):
    #   echo $GITHUB_PAT | helm registry login ghcr.io -u <user> --password-stdin   (PAT with read:packages)
    helm install project-developer oci://ghcr.io/rehn/charts/project-developer --version 0.1.0 \
      --namespace project-developer --create-namespace -f my-values.yaml

Example `my-values.yaml`:

    httpRoute:
      enabled: true
      parentRefs: [{ name: my-gateway, namespace: gateways, sectionName: https }]
      hostnames: [project-developer.example.com]
      # timeouts: { request: 0s }   # if your gateway times out long-lived requests
    persistence:
      size: 20Gi
      storageClass: ""          # "" = cluster default
    config:
      git: { authorName: "Your Name", authorEmail: "you@example.com" }
    # imagePullSecrets: [{ name: dockerhub-pull }]   # only if the Docker Hub repo is private

- **Exposing it:** the chart exposes the app only through a Gateway API `HTTPRoute` (`httpRoute.parentRefs` is required); there is no Ingress support. Without a route, use `kubectl port-forward`. The terminal is a long-lived WebSocket, so make sure your gateway doesn't cut idle connections too early (implementation specific, e.g. an Envoy Gateway `ClientTrafficPolicy`/`BackendTrafficPolicy`). Image uploads in the terminal are up to 20 MB; Envoy-based gateways and Traefik have no body limit by default, NGINX Gateway Fabric needs a `ClientSettingsPolicy` with `body.maxSize`.
- **Storage:** a PVC (default 20Gi, `ReadWriteOnce`) is mounted at `/data` and holds everything that must be kept: the database and its encryption key, cloned repos, and Claude's login/history/usage logs. Use `persistence.existingClaim` to bring your own, or add `persistence.annotations: {helm.sh/resource-policy: keep}` so the data survives `helm uninstall`.
- **One replica only:** state is SQLite on the volume plus live terminal sessions in memory.
- **From a checkout** (no registry): `helm install project-developer ./charts/project-developer -f my-values.yaml`.
- **First run:** create your account, add your GitHub token in Settings (it is also given to `gh`/`git push` inside Claude sessions), then Terminal -> **Sign in with Claude**. There's no browser in the pod: open the printed link on your own machine and paste the code back into the terminal.
- **Security:** anyone who can sign in gets Claude with shell access inside the pod. Keep the pod's network access and credentials minimal and put the gateway behind a VPN or SSO if you can.
