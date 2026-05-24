-- name: UpsertSource :exec
INSERT INTO sources(id, description, first_seen, last_seen)
VALUES (?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET last_seen = excluded.last_seen;

-- name: GetSource :one
SELECT id, description, first_seen, last_seen FROM sources WHERE id = ?;

-- name: ListSources :many
SELECT id, description, first_seen, last_seen FROM sources ORDER BY id;

-- name: UpdateSource :exec
UPDATE sources SET description = ? WHERE id = ?;

-- name: DeleteSource :exec
DELETE FROM sources WHERE id = ?;

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
INSERT INTO changes(entity, entity_id, source, op, revision, at) VALUES (?, ?, ?, ?, ?, ?);

-- name: CreateNode :exec
INSERT INTO nodes(id, domain, layer, name, parent_id, source, properties, revision, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?);

-- name: GetNode :one
SELECT id, domain, layer, name, parent_id, source, properties, revision, created_at, updated_at
FROM nodes WHERE id = ?;

-- name: ListNodes :many
SELECT id, domain, layer, name, parent_id, source, properties, revision, created_at, updated_at
FROM nodes
WHERE (sqlc.arg(domain_filter) = '' OR domain = sqlc.arg(domain_filter))
  AND (sqlc.arg(layer_filter)  = '' OR layer  = sqlc.arg(layer_filter))
  AND (sqlc.arg(source_filter) = '' OR source = sqlc.arg(source_filter))
ORDER BY id
LIMIT CASE WHEN sqlc.arg(lim) = 0 THEN -1 ELSE sqlc.arg(lim) END;

-- name: ChildrenOf :many
SELECT id, domain, layer, name, parent_id, source, properties, revision, created_at, updated_at
FROM nodes WHERE parent_id = ? ORDER BY id;

-- name: NodesOwnedBy :many
SELECT id, domain, layer, name, parent_id, source, properties, revision, created_at, updated_at
FROM nodes WHERE domain = ? AND source = ? ORDER BY id;

-- name: UpdateNode :exec
UPDATE nodes SET name = ?, properties = ?, revision = revision + 1, updated_at = ?
WHERE id = ?;

-- name: GetNodeRevision :one
SELECT revision FROM nodes WHERE id = ?;

-- name: DeleteNode :exec
DELETE FROM nodes WHERE id = ?;

-- name: UpsertEdge :one
INSERT INTO edges(source_id, target_id, type, properties, revision, created_at)
VALUES (?, ?, ?, ?, 1, ?)
ON CONFLICT(source_id, target_id, type) DO UPDATE SET source_id = excluded.source_id
RETURNING id;

-- name: GetEdge :one
SELECT id, source_id, target_id, type, properties, revision, created_at FROM edges WHERE id = ?;

-- name: UpdateEdgeProperties :exec
UPDATE edges SET properties = ?, revision = revision + 1 WHERE id = ?;

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

-- name: AddEdgeClaim :exec
INSERT OR IGNORE INTO edge_claims(edge_id, source, claimed_at) VALUES (?, ?, ?);

-- name: RemoveEdgeClaim :exec
DELETE FROM edge_claims WHERE edge_id = ? AND source = ?;

-- name: CountEdgeClaims :one
SELECT COUNT(*) AS n FROM edge_claims WHERE edge_id = ?;

-- name: ListEdgeClaims :many
SELECT edge_id, source, claimed_at FROM edge_claims WHERE edge_id = ? ORDER BY source;

-- name: EdgeIDsClaimedBy :many
SELECT edge_id FROM edge_claims WHERE source = ? ORDER BY edge_id;
