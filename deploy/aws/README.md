# RelayForge AWS deployment

The `relayforge-prod` CloudFormation stack runs RelayForge on one
`m7i-flex.large` EC2 host in `eu-north-1`. CodePipeline reads
`FinzyPHINZY/relayforge` `main` through AWS CodeConnections. Each main commit
is sent to CodeDeploy, whose `AfterInstall` hook runs the Go, dashboard, and
SDK checks on the host. Only passing checks start the Compose services; a
health check then completes the deployment. The instance keeps PostgreSQL data
in an encrypted EBS root volume and has no SSH ingress.

GitHub branch protection requires the `test` check before a pull request can
merge. On each push to `main`, the GitHub CI `deploy` job starts the AWS pipeline
for that exact checked commit only after its `test` job passes, then waits for the deployment result. The
`relayforge-ci-trigger` service identity can only start and read this pipeline;
its policy is in `deploy/aws/ci-trigger-policy.json`, and its access key is
stored in GitHub Actions repository secrets. Rotate that key through IAM and
update both repository secrets when needed.

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

3. In Namecheap, set the domain's nameservers to `ns-622.awsdns-13.net`,
   `ns-1034.awsdns-01.org`, `ns-1848.awsdns-39.co.uk`, and
   `ns-237.awsdns-29.com`. Route 53 already contains A records for `relayforge.site` and
   `api.relayforge.site` pointing to the stack's Elastic IP. Wait for DNS to
   propagate; Caddy obtains HTTPS certificates automatically.
4. Merge this deployment configuration to `main`. The pipeline will check and
   deploy the merged commit. Check `relayforge-prod-pipeline` in CodePipeline
   and the CodeDeploy deployment event logs.
5. Verify `https://relayforge.site/` and
   `https://api.relayforge.site/health/ready`.

## Console sign-in setup

Console sign-in (GitHub and emailed sign-in links) is off until
`DASHBOARD_PUBLIC_URL`, `DASHBOARD_API_TOKEN` and `IP_HASH_KEY_BASE64` are set;
until then the console offers API-key login only. These steps are manual and
are done by the project owner.

1. **Create the GitHub OAuth App.** In GitHub → Settings → Developer settings →
   OAuth Apps → New OAuth App, set the homepage URL to
   `https://relayforge.site` and the authorization callback URL to
   `https://relayforge.site/auth/github/callback`. Generate a client secret.
   For local development create a separate app with callback
   `http://localhost:3000/auth/github/callback`.
2. **Add the sign-in settings to the host env file.** Open a Session Manager
   shell on the instance and *append* the variables below to
   `/etc/relayforge/relayforge.env`. Do not replace the file, and never change
   `ENCRYPTION_KEY_BASE64`: existing endpoint secrets are encrypted with it.
   Generate each secret separately with `openssl rand -base64 32`.

   ```sh
   DASHBOARD_PUBLIC_URL=https://relayforge.site
   DASHBOARD_API_TOKEN=<openssl rand -base64 32>
   IP_HASH_KEY_BASE64=<openssl rand -base64 32>
   MAIL_PROVIDER=ses
   MAIL_FROM=RelayForge <sign-in@relayforge.site>
   AWS_REGION=eu-north-1
   MAIL_LOG_BODIES=false
   GITHUB_CLIENT_ID=<from step 1>
   GITHUB_CLIENT_SECRET=<from step 1>
   MAGIC_LINK_PER_EMAIL_PER_HOUR=5
   MAGIC_LINK_PER_IP_PER_HOUR=20
   OAUTH_START_PER_IP_PER_HOUR=30
   MAX_WORKSPACES_PER_USER=3
   ```

   Keep the file mode `0600`. Do not add AWS access keys: the API container
   gets SES credentials from the instance role through IMDSv2 (the instance's
   metadata hop limit is 2 so containers can reach it). The send permission is
   limited to `ses:SendEmail` on the `relayforge.site` identity with the From
   address `sign-in@relayforge.site`.
3. **Deploy the stack** with the command under [Maintenance](#maintenance).
   It creates the SES domain identity (Easy DKIM, MAIL FROM domain
   `mail.relayforge.site`) and adds these records to the Route 53 zone: three
   DKIM CNAMEs (also shown in the `MailDkimCname1..3` stack outputs), the MAIL
   FROM MX `10 feedback-smtp.eu-north-1.amazonses.com` and SPF TXT
   `v=spf1 include:amazonses.com ~all`, and a monitoring-only DMARC record
   `v=DMARC1; p=none;` at `_dmarc.relayforge.site`. If you move DNS off
   Route 53, copy those records to the new DNS provider. If an SES identity for
   `relayforge.site` was already created by hand, the stack update fails;
   delete that identity first or import it into the stack.
4. **Wait for verification.** Repeat until `DkimAttributes.Status` and
   `MailFromAttributes.MailFromDomainStatus` are both `SUCCESS` (usually
   minutes, up to 72 hours):

   ```sh
   aws sesv2 get-email-identity --email-identity relayforge.site \
     --profile project --region eu-north-1
   ```

5. **Request SES production access.** New SES accounts are in the sandbox:
   they send only to verified addresses, so sign-in links to other people are
   rejected. Request production access for `eu-north-1` in the SES console
   (Account dashboard → Request production access) or with
   `aws sesv2 put-account-details --production-access-enabled --mail-type TRANSACTIONAL --website-url https://relayforge.site --use-case-description "<how sign-in email is used>" --profile project --region eu-north-1`.
   AWS reviews the request, typically within a day. Until it is granted,
   verify your own address with
   `aws sesv2 create-email-identity --email-identity <you@example.com> --profile project --region eu-north-1`
   to test sign-in.
6. **Deploy the application** by merging to `main` (see One-time setup step 4).
   The Compose file passes the new variables to the API and dashboard.
7. **Link the bootstrap organization to your email.** On the host, find the
   organization ID and link yourself as its owner:

   ```sh
   cd /opt/relayforge/current
   docker compose --env-file /etc/relayforge/relayforge.env -p relayforge -f deploy/compose.yaml \
     exec -T postgres psql -U webhooks -d webhooks -c 'SELECT id, name FROM organizations'
   docker compose --env-file /etc/relayforge/relayforge.env -p relayforge -f deploy/compose.yaml \
     --profile ops run --rm tenant-admin -action link-owner -org-id <id> -email <addr>
   ```

8. **Sign in** at `https://relayforge.site/login` with GitHub (using an
   account whose verified primary email is `<addr>`) or by requesting an email link.

Caddy sets `X-Forwarded-For` to the connecting client (it trusts no upstream
proxy) and strips client-supplied `X-RelayForge-Dashboard-Token` and
`X-RelayForge-Client-IP` headers. The dashboard reaches the API at
`http://api:8080` on the Compose network, and the API port is bound to
`127.0.0.1` on the host, so the API is reachable from outside only through
Caddy.

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

Use Systems Manager Session Manager for maintenance. For operator access, run
`cmd/bootstrap` inside the API image on the instance and store the resulting
API key privately; never put it in GitHub or CloudFormation parameters.
