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

# ---- 3. Toolchains for the runtime image, taken for the *target* architecture ----
# Same Debian release as the runtime stage, so the binaries are glibc-compatible.
FROM golang:1.26-bookworm AS go-toolchain
FROM node:22-bookworm-slim AS node-toolchain
FROM alpine/helm:3.19.0 AS helm-toolchain
FROM registry.k8s.io/kubectl:v1.34.1 AS kubectl-toolchain

# ---- 4. Runtime: Debian + git + GitHub CLI + Go + Node.js + Python + Helm + kubectl + Claude Code ----
FROM debian:bookworm-slim
# Pin Claude Code; bump to upgrade (see https://claude.com/claude-code).
ARG CLAUDE_CODE_VERSION=2.1.326

RUN apt-get update \
 && apt-get install -y --no-install-recommends bubblewrap ca-certificates curl git openssh-client python3 python3-pip python3-venv tini \
 # GitHub CLI from the official apt repo
 && mkdir -p -m 755 /etc/apt/keyrings \
 && curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg -o /etc/apt/keyrings/githubcli-archive-keyring.gpg \
 && chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg \
 && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" > /etc/apt/sources.list.d/github-cli.list \
 && apt-get update && apt-get install -y --no-install-recommends gh \
 && rm -rf /var/lib/apt/lists/* \
 && python3 --version && pip3 --version

# Go and Node.js (+ npm, corepack), so Claude can build and test projects inside the container.
COPY --from=go-toolchain /usr/local/go /usr/local/go
COPY --from=node-toolchain /usr/local/bin/node /usr/local/bin/node
COPY --from=node-toolchain /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
 && ln -s ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx \
 && ln -s ../lib/node_modules/corepack/dist/corepack.js /usr/local/bin/corepack \
 && ln -s /usr/local/go/bin/go /usr/local/bin/go \
 && ln -s /usr/local/go/bin/gofmt /usr/local/bin/gofmt \
 && node --version && npm --version && go version

# Helm (statically linked, so the binary from the Alpine-based image runs fine on Debian).
COPY --from=helm-toolchain /usr/bin/helm /usr/local/bin/helm
RUN helm version --short

# kubectl (static Go binary from the official image; multi-arch). Cluster access comes from a kubeconfig uploaded in Settings.
COPY --from=kubectl-toolchain /bin/kubectl /usr/local/bin/kubectl
RUN kubectl version --client

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
