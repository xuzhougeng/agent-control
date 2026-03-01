#!/usr/bin/env bash
# Deploy cc-control server to remote host.
# Usage:
#   bash scripts/deploy-server.sh             # full deploy (control binary + web)
#   bash scripts/deploy-server.sh control     # cc-control binary only (restarts service)
#   bash scripts/deploy-server.sh web         # cc-web static files only (no restart)
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REMOTE_HOST="${REMOTE_HOST:-ubuntu@106.54.201.18}"
SERVER_DIR="$REPO_ROOT/dist/multichat-release/server"

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

rsync_to_opt() { rsync -av -e ssh --rsync-path="sudo rsync" "$@"; }

case "$MODE" in
  control)
    echo "==> Rsync cc-control binary to $REMOTE_HOST"
    rsync_to_opt "$SERVER_DIR/bin/cc-control" "$REMOTE_HOST:/opt/cc-control/cc-control"
    echo "==> Restarting cc-control on remote"
    ssh "$REMOTE_HOST" sudo systemctl restart cc-control
    echo "==> Deploy complete (control only), cc-control restarted"
    ;;

  web)
    echo "==> Rsync cc-web to $REMOTE_HOST"
    rsync_to_opt --delete "$SERVER_DIR/cc-web/" "$REMOTE_HOST:/opt/cc-control/cc-web/"
    echo "==> Deploy complete (web only), no restart needed"
    ;;

  full)
    echo "==> Rsync cc-control binary and cc-web to $REMOTE_HOST"
    rsync_to_opt "$SERVER_DIR/bin/cc-control" "$REMOTE_HOST:/opt/cc-control/cc-control"
    rsync_to_opt --delete "$SERVER_DIR/cc-web/" "$REMOTE_HOST:/opt/cc-control/cc-web/"
    echo "==> Restarting cc-control on remote"
    ssh "$REMOTE_HOST" sudo systemctl restart cc-control
    echo "==> Deploy complete (full), cc-control restarted"
    ;;
esac
