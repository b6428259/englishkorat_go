#!/usr/bin/env bash
set -euo pipefail

# host-cleanup.sh
# Safe(ish) disk cleanup for small EC2 instances
# - Prunes unused Docker resources (images/containers/networks/volumes)
# - Prunes Docker builder cache
# - Truncates oversized Docker JSON logs
# - Cleans apt caches and vacuums journald logs

log() { printf "[%s] %s\n" "cleanup" "$*" >&2; }

BYTES_BEFORE=$(df -B1 / | awk 'NR==2{print $3}')
log "Disk usage before:"
df -h /

log "Docker disk usage before:"
docker system df || true

# Prune stopped/exited containers (safe)
log "Pruning stopped containers..."
docker container prune -f || true

# Remove dangling images and any image not used by a container
log "Pruning docker images (aggressive)..."
docker image prune -af || true

# Remove dangling/unused volumes (keeps volumes used by running containers)
log "Pruning docker volumes (unused)..."
docker volume prune -f || true

# Remove unused networks
log "Pruning docker networks (unused)..."
docker network prune -f || true

# Prune builder cache
log "Pruning docker builder cache..."
docker builder prune -af || true

# Truncate huge Docker JSON logs (do not remove files, just truncate)
log "Truncating large docker container logs (>50MB) ..."
sudo find /var/lib/docker/containers -type f -name '*-json.log' -size +50M -exec sh -c 'truncate -s 0 "$1" && echo "Truncated: $1"' _ {} \; 2>/dev/null || true

# Clean apt cache
log "Cleaning apt caches..."
if command -v apt-get >/dev/null 2>&1; then
  sudo apt-get autoremove -y || true
  sudo apt-get clean || true
fi

# Vacuum journald logs (keep 7 days)
log "Vacuuming journald (7d)..."
sudo journalctl --vacuum-time=7d >/dev/null 2>&1 || true

log "Docker disk usage after:"
docker system df || true

log "Disk usage after:"
df -h /

BYTES_AFTER=$(df -B1 / | awk 'NR==2{print $3}')
FREED=$(( BYTES_BEFORE - BYTES_AFTER ))
if [ $FREED -gt 0 ]; then
  log "Freed $(numfmt --to=iec --suffix=B $FREED 2>/dev/null || echo ${FREED}B)"
else
  log "No additional space freed (or increased usage due to deploy)."
fi

exit 0
