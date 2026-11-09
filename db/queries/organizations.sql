-- name: CreateOrganization :exec
INSERT INTO organizations (id, name) VALUES ($1, $2);

-- name: GetActiveOrganization :one
SELECT id, name, monthly_event_quota, max_endpoints FROM organizations
WHERE id = $1 AND status = 'active';
