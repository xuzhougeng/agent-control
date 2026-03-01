#!/usr/bin/env bash
# Deploy cc-control server to remote host.
# Usage: bash scripts/deploy-server.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REMOTE_HOST="${REMOTE_HOST:-ubuntu@106.54.201.18}"
REMOTE_TMP="/home/ubuntu/server.tar.gz"
ARTIFACT="$REPO_ROOT/dist/multichat-release/artifacts/server.tar.gz"

echo "==> Building release bundle"
bash "$REPO_ROOT/scripts/build-release-bundle.sh"

echo "==> Uploading server.tar.gz to $REMOTE_HOST"
scp "$ARTIFACT" "$REMOTE_HOST:$REMOTE_TMP"

echo "==> Deploying on remote host"
ssh "$REMOTE_HOST" bash -s <<'REMOTE_SCRIPT'
set -euo pipefail
cd /home/ubuntu
tar xf server.tar.gz
sudo mv server/bin/cc-control /opt/cc-control/cc-control
sudo rsync -av --delete server/cc-web/ /opt/cc-control/cc-web/
rm -rf server server.tar.gz
echo "==> Deploy complete, cc-control restarted"
REMOTE_SCRIPT
