#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="/opt/englishkorat"
cd "$REPO_DIR"

echo "[1/6] Updating repo" >&2
git fetch origin main
git reset --hard origin/main

# Ensure aws cli installed
if ! command -v aws >/dev/null 2>&1; then
	echo "[2/6] Installing awscli" >&2
	sudo apt-get update -y && sudo apt-get install -y awscli
else
	echo "[2/6] awscli present" >&2
fi

echo "[3/6] Generating .env from SSM" >&2
bash deploy/generate-env.sh

SHORT_SHA=$(git rev-parse --short HEAD)
export IMAGE_TAG=$SHORT_SHA

echo "[4/6] Pulling images (tag: $IMAGE_TAG)" >&2
docker compose -f docker-compose.production.yml pull || true

echo "[5/6] Starting services" >&2
docker compose -f docker-compose.production.yml up -d --remove-orphans

echo "[6/6] Cleaning old images" >&2
docker image prune -f >/dev/null 2>&1 || true

echo "Deployment complete." >&2
