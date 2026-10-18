#!/usr/bin/env bash
# Cross-compiles the Go binary for the HA app (amd64 + aarch64).
# Pure-Go deps (modernc sqlite) → CGO_ENABLED=0, no emulation needed.
set -euo pipefail
cd "$(dirname "$0")/.."

mkdir -p dist
for arch in amd64 arm64; do
  GOOS=linux GOARCH=$arch CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w" -o "dist/agentrelay-$arch" ./cmd/agentrelay
  echo "built dist/agentrelay-$arch"
done
# stage into the add-on dir: the Supervisor build context is agent-relay/
# (review-2 copilot H-7); repo-root dist/ stays the canonical output too.
mkdir -p agent-relay/dist
cp dist/agentrelay-amd64 dist/agentrelay-arm64 agent-relay/dist/
ls -la agent-relay/dist/