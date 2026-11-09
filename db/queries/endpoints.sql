-- name: GetEndpoint :one
SELECT id, organization_id, url, description, status, timeout_ms, max_attempts, created_at, updated_at
FROM endpoints WHERE id = $1 AND organization_id = $2;

-- name: ListEndpoints :many
SELECT id, organization_id, url, description, status, timeout_ms, max_attempts, created_at, updated_at
FROM endpoints WHERE organization_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2;
