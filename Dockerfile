# syntax=docker/dockerfile:1

# Stages 1 and 2 always run on the build machine's native architecture ($BUILDPLATFORM)
# and cross-compile, so multi-arch builds never emulate (QEMU) the slow compile steps.

# ---- 1. Frontend: static export of the Next.js app (architecture independent) ----
FROM --platform=$BUILDPLATFORM node:22-bookworm-slim AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
ENV NEXT_EXPORT=1 NEXT_TELEMETRY_DISABLED=1
RUN npm run build            # -> /web/out

# ---- 2. Backend: static Go binary (pure-Go SQLite, no cgo), cross-compiled ----
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS backend
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/project-developer .

# ---- 3. Runtime: Debian + git + GitHub CLI + Claude Code ----
FROM debian:bookworm-slim
# Pin Claude Code; bump to upgrade (see https://claude.com/claude-code).
ARG CLAUDE_CODE_VERSION=2.1.326

RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl git openssh-client tini \
 # GitHub CLI from the official apt repo
 && mkdir -p -m 755 /etc/apt/keyrings \
 && curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg -o /etc/apt/keyrings/githubcli-archive-keyring.gpg \
 && chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg \
 && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" > /etc/apt/sources.list.d/github-cli.list \
 && apt-get update && apt-get install -y --no-install-recommends gh \
 && rm -rf /var/lib/apt/lists/*

# Claude Code (native installer), installed system-wide so a mounted volume can't hide it.
RUN HOME=/tmp/claude-install curl -fsSL https://claude.ai/install.sh | HOME=/tmp/claude-install bash -s "${CLAUDE_CODE_VERSION}" \
 && cp -L /tmp/claude-install/.local/bin/claude /usr/local/bin/claude \
 && rm -rf /tmp/claude-install \
 && claude --version

RUN useradd --uid 1000 --create-home --shell /bin/bash app \
 && mkdir -p /data /app && chown app:app /data
COPY --from=backend /out/project-developer /app/project-developer
COPY --from=web /web/out /app/web

USER app
# HOME lives on the volume so Claude's login, history and usage logs survive restarts.
ENV HOME=/data/home \
    DATA_DIR=/data \
    STATIC_DIR=/app/web \
    ADDR=0.0.0.0:8080 \
    DISABLE_AUTOUPDATER=1 \
    CLAUDE_BIN=/usr/local/bin/claude
WORKDIR /data
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD curl -fs http://127.0.0.1:8080/healthz || exit 1
# tini reaps the child processes Claude spawns.
ENTRYPOINT ["tini", "--", "/app/project-developer"]
