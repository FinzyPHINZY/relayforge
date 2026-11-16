#!/usr/bin/env bash
set -euo pipefail

release_sha="${CODEBUILD_RESOLVED_SOURCE_VERSION:?source SHA required}"
release_bucket="${RELEASE_BUCKET:?release bucket required}"
instance_id="${INSTANCE_ID:?instance ID required}"
archive="/tmp/relayforge-${release_sha}.tar.gz"

tar -czf "$archive" --exclude='web/dashboard/node_modules' --exclude='web/dashboard/.next' -C "$CODEBUILD_SRC_DIR" .
aws s3 cp "$archive" "s3://${release_bucket}/releases/${release_sha}.tar.gz" --region eu-north-1
aws s3 cp deploy/aws/remote-deploy.sh "s3://${release_bucket}/bootstrap/remote-deploy.sh" --region eu-north-1

python3 - <<'PY'
import json
import os

bucket = os.environ["RELEASE_BUCKET"]
sha = os.environ["CODEBUILD_RESOLVED_SOURCE_VERSION"]
instance = os.environ["INSTANCE_ID"]
request = {
    "InstanceIds": [instance],
    "DocumentName": "AWS-RunShellScript",
    "Comment": f"Deploy RelayForge {sha}",
    "Parameters": {"commands": [
        f"aws s3 cp s3://{bucket}/bootstrap/remote-deploy.sh /opt/relayforge/deploy-remote.sh --region eu-north-1",
        f"bash /opt/relayforge/deploy-remote.sh {sha} {bucket}",
    ]},
}
with open("/tmp/relayforge-ssm-command.json", "w", encoding="utf-8") as file:
    json.dump(request, file)
PY

command_id="$(aws ssm send-command \
  --region eu-north-1 \
  --cli-input-json file:///tmp/relayforge-ssm-command.json \
  --query 'Command.CommandId' --output text)"

for attempt in $(seq 1 120); do
  status="$(aws ssm get-command-invocation --region eu-north-1 --command-id "$command_id" --instance-id "$instance_id" --query Status --output text 2>/dev/null || true)"
  case "$status" in
    Success) exit 0 ;;
    Failed|Cancelled|TimedOut)
      aws ssm get-command-invocation --region eu-north-1 --command-id "$command_id" --instance-id "$instance_id" --query '{Status:Status,Output:StandardOutputContent,Error:StandardErrorContent}' --output json
      exit 1
      ;;
  esac
  sleep 10
done
echo "Deployment command did not finish within 20 minutes" >&2
exit 1
