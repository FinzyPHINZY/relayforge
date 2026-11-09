-- name: GetEvent :one
SELECT id, organization_id, event_type, payload, created_at FROM events
WHERE id = $1 AND organization_id = $2;

-- name: ListEvents :many
SELECT id, organization_id, event_type, created_at FROM events
WHERE organization_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2;
