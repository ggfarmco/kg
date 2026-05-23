-- name: CreateDomain :exec
INSERT INTO domains(id, description, layers, revision, created_at)
VALUES (?, ?, ?, 1, ?);

-- name: GetDomain :one
SELECT id, description, layers, revision, created_at FROM domains WHERE id = ?;

-- name: ListDomains :many
SELECT id, description, layers, revision, created_at FROM domains ORDER BY id;

-- name: DeleteDomain :exec
DELETE FROM domains WHERE id = ?;

-- name: AppendChange :exec
INSERT INTO changes(entity, entity_id, op, revision, at) VALUES (?, ?, ?, ?, ?);

-- name: CreateNode :exec
INSERT INTO nodes(id, domain, layer, name, parent_id, summary, properties, revision, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?);

-- name: GetNode :one
SELECT id, domain, layer, name, parent_id, summary, properties, revision, created_at, updated_at
FROM nodes WHERE id = ?;

-- name: ListNodes :many
SELECT id, domain, layer, name, parent_id, summary, properties, revision, created_at, updated_at
FROM nodes
WHERE (sqlc.arg(domain_filter) = '' OR domain = sqlc.arg(domain_filter))
  AND (sqlc.arg(layer_filter)  = '' OR layer  = sqlc.arg(layer_filter))
ORDER BY id
LIMIT CASE WHEN sqlc.arg(lim) = 0 THEN -1 ELSE sqlc.arg(lim) END;

-- name: ChildrenOf :many
SELECT id, domain, layer, name, parent_id, summary, properties, revision, created_at, updated_at
FROM nodes WHERE parent_id = ? ORDER BY id;

-- name: UpdateNode :exec
UPDATE nodes SET name = ?, summary = ?, properties = ?, revision = revision + 1, updated_at = ?
WHERE id = ?;

-- name: GetNodeRevision :one
SELECT revision FROM nodes WHERE id = ?;

-- name: DeleteNode :exec
DELETE FROM nodes WHERE id = ?;

-- name: CreateEdge :one
INSERT INTO edges(source_id, target_id, type, properties, revision, created_at)
VALUES (?, ?, ?, ?, 1, ?) RETURNING id;

-- name: GetEdge :one
SELECT id, source_id, target_id, type, properties, revision, created_at FROM edges WHERE id = ?;

-- name: DeleteEdge :exec
DELETE FROM edges WHERE id = ?;

-- name: EdgesFromAll :many
SELECT id, source_id, target_id, type, properties, revision, created_at
FROM edges WHERE source_id = ? ORDER BY id;

-- name: EdgesFromTyped :many
SELECT id, source_id, target_id, type, properties, revision, created_at
FROM edges WHERE source_id = ? AND type IN (sqlc.slice(types)) ORDER BY id;

-- name: EdgesToAll :many
SELECT id, source_id, target_id, type, properties, revision, created_at
FROM edges WHERE target_id = ? ORDER BY id;

-- name: EdgesToTyped :many
SELECT id, source_id, target_id, type, properties, revision, created_at
FROM edges WHERE target_id = ? AND type IN (sqlc.slice(types)) ORDER BY id;
