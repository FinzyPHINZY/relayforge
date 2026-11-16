#!/usr/bin/env bash
set -euo pipefail
cd /opt/relayforge/current

docker run --rm --network host \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$PWD":/work -w /work \
  golang:1.25 sh -c 'go mod download && go vet ./... && go test ./...'

docker run --rm \
  -v "$PWD":/work -w /work \
  node:22 sh -c 'corepack enable && cd web/dashboard && pnpm install --frozen-lockfile && pnpm test && pnpm typecheck && pnpm build && cd ../.. && web/dashboard/node_modules/.bin/tsc -p sdk/typescript/tsconfig.json && node sdk/typescript/test/client.test.mjs'
