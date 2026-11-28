#!/usr/bin/env bash
set -euo pipefail
cd /opt/relayforge/current
docker compose --env-file /etc/relayforge/relayforge.env -p relayforge -f deploy/compose.yaml up --build -d --remove-orphans
# every deploy rebuilds the images; without this the build cache filled the 50 GB disk.
# runs only after a successful start; keeps one day of cache so rebuilds stay fast.
docker builder prune -af --filter until=24h >/dev/null
docker image prune -f >/dev/null
