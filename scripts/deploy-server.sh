#!/usr/bin/env bash
# Deploy cc-control server to remote host.
# Usage:
#   bash scripts/deploy-server.sh             # full deploy (control binary + web)
#   bash scripts/deploy-server.sh control     # cc-control binary only (restarts service)
#   bash scripts/deploy-server.sh web         # cc-web static files only (no restart)
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REMOTE_HOST="${REMOTE_HOST:-ubuntu@106.54.201.18}"
REMOTE_TMP="/home/ubuntu/server.tar.gz"
ARTIFACT="$REPO_ROOT/dist/multichat-release/artifacts/server.tar.gz"

MODE="${1:-full}"

case "$MODE" in
  control|web|full) ;;
  *)
    echo "Usage: $0 [control|web|full]" >&2
    exit 1
    ;;
esac

echo "==> Building release bundle (mode: $MODE)"
bash "$REPO_ROOT/scripts/build-release-bundle.sh"

echo "==> Uploading server.tar.gz to $REMOTE_HOST"
scp "$ARTIFACT" "$REMOTE_HOST:$REMOTE_TMP"

echo "==> Deploying on remote host (mode: $MODE)"

case "$MODE" in
  control)
    ssh "$REMOTE_HOST" bash -s <<'REMOTE_SCRIPT'
set -euo pipefail
cd /home/ubuntu
tar xf server.tar.gz
sudo mv server/bin/cc-control /opt/cc-control/cc-control
rm -rf server server.tar.gz
sudo systemctl restart cc-control
echo "==> Deploy complete (control only), cc-control restarted"
REMOTE_SCRIPT
    ;;

  web)
    ssh "$REMOTE_HOST" bash -s <<'REMOTE_SCRIPT'
set -euo pipefail
cd /home/ubuntu
tar xf server.tar.gz
sudo rsync -av --delete server/cc-web/ /opt/cc-control/cc-web/
rm -rf server server.tar.gz
echo "==> Deploy complete (web only), no restart needed"
REMOTE_SCRIPT
    ;;

  full)
    ssh "$REMOTE_HOST" bash -s <<'REMOTE_SCRIPT'
set -euo pipefail
cd /home/ubuntu
tar xf server.tar.gz
sudo mv server/bin/cc-control /opt/cc-control/cc-control
sudo rsync -av --delete server/cc-web/ /opt/cc-control/cc-web/
rm -rf server server.tar.gz
sudo systemctl restart cc-control
echo "==> Deploy complete (full), cc-control restarted"
REMOTE_SCRIPT
    ;;
esac
