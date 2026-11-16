#!/usr/bin/env bash
set -euo pipefail

release_sha="${1:?release SHA required}"
release_dir="/opt/relayforge/releases/${release_sha}"
archive="/tmp/relayforge-${release_sha}.tar.gz"
bucket="${2:?release bucket required}"

mkdir -p "$release_dir"
aws s3 cp "s3://${bucket}/releases/${release_sha}.tar.gz" "$archive" --region eu-north-1
tar -xzf "$archive" -C "$release_dir"
rm -f "$archive"

cd "$release_dir"
docker compose --env-file /etc/relayforge/relayforge.env -p relayforge -f deploy/compose.yaml up --build -d --remove-orphans
for attempt in $(seq 1 60); do
  if curl --fail --silent http://127.0.0.1:8080/health/ready >/dev/null && curl --fail --silent http://127.0.0.1:3000/login >/dev/null; then
    printf '%s\n' "$release_sha" >/opt/relayforge/current-sha
    exit 0
  fi
  sleep 5
done
docker compose --env-file /etc/relayforge/relayforge.env -p relayforge -f deploy/compose.yaml ps
exit 1
