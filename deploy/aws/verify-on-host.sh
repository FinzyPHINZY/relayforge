#!/usr/bin/env bash
set -euo pipefail
for attempt in $(seq 1 60); do
  if curl --fail --silent http://127.0.0.1:8080/health/ready >/dev/null && curl --fail --silent http://127.0.0.1:3000/login >/dev/null; then
    exit 0
  fi
  sleep 5
done
cd /opt/relayforge/current
docker compose --env-file /etc/relayforge/relayforge.env -p relayforge -f deploy/compose.yaml ps
exit 1
