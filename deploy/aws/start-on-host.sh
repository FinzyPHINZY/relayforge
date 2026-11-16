#!/usr/bin/env bash
set -euo pipefail
cd /opt/relayforge/current
docker compose --env-file /etc/relayforge/relayforge.env -p relayforge -f deploy/compose.yaml up --build -d --remove-orphans
