# RelayForge AWS deployment

The `relayforge-prod` CloudFormation stack runs RelayForge on one EC2 host in
`eu-north-1`. Its CodePipeline source is `FinzyPHINZY/relayforge` `main` through
AWS CodeConnections. Each main commit runs the Go, dashboard, and SDK checks in
CodeBuild. Only a passing build uploads that commit's source archive and sends a
Systems Manager deployment command to the host. The instance keeps PostgreSQL
data in an encrypted EBS root volume and has no SSH ingress.

## One-time setup

1. Confirm the selected Region under AWS Settings → View all projects → Overview
   → Additional Info → Region. The `project` CLI profile should show `eu-north-1`.
2. Authorize the `relayforge-github` CodeConnections connection in the AWS
   Management Console, then verify its status is `AVAILABLE`:

   ```sh
   aws codeconnections get-connection \
     --connection-arn arn:aws:codeconnections:eu-north-1:895207966063:connection/1ace6d55-1daa-4ea7-a839-da8ef25d42c1 \
     --profile project --region eu-north-1
   ```

3. In Namecheap, set the domain's nameservers to the stack's `NameServers`
   output. Route 53 already contains A records for `relayforge.site` and
   `api.relayforge.site` pointing to the stack's Elastic IP. Wait for DNS to
   propagate; Caddy obtains HTTPS certificates automatically.
4. Merge this deployment configuration to `main`. The pipeline will check and
   deploy the merged commit. Check `relayforge-prod-pipeline` in CodePipeline
   and the `relayforge-prod-checks` CodeBuild logs.
5. Verify `https://relayforge.site/` and
   `https://api.relayforge.site/health/ready`.

## Maintenance

The AWS CLI default profile is a different project. Always pass
`--profile project --region eu-north-1` for this deployment. Stack source is
`deploy/aws/stack.yaml`; apply changes with:

```sh
aws cloudformation deploy --template-file deploy/aws/stack.yaml \
  --stack-name relayforge-prod --capabilities CAPABILITY_IAM \
  --parameter-overrides GitHubConnectionArn=arn:aws:codeconnections:eu-north-1:895207966063:connection/1ace6d55-1daa-4ea7-a839-da8ef25d42c1 \
  --profile project --region eu-north-1
```

Application secrets are generated on the instance in
`/etc/relayforge/relayforge.env` and `/etc/relayforge/postgres_password` with
mode `0600`. Back up the PostgreSQL volume and the encryption key together;
the single EC2 host is not a high-availability deployment. Do not delete the
instance or root volume without first exporting and verifying a database backup.

Use Systems Manager Session Manager for maintenance. The current deployed Git
SHA is recorded at `/opt/relayforge/current-sha`. For operator access, run
`cmd/bootstrap` inside the API image on the instance and store the resulting
API key privately; never put it in GitHub or CloudFormation parameters.
