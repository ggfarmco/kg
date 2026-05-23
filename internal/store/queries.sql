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
