-- +goose Up
CREATE TABLE domains (
  id          TEXT PRIMARY KEY,
  description TEXT,
  layers      TEXT NOT NULL,
  revision    INTEGER NOT NULL DEFAULT 1,
  created_at  INTEGER NOT NULL
);

CREATE TABLE nodes (
  id          TEXT PRIMARY KEY,
  domain      TEXT NOT NULL REFERENCES domains(id) ON DELETE RESTRICT,
  layer       TEXT NOT NULL,
  name        TEXT NOT NULL,
  parent_id   TEXT REFERENCES nodes(id) ON DELETE RESTRICT,
  summary     TEXT,
  properties  TEXT NOT NULL DEFAULT '{}',
  revision    INTEGER NOT NULL DEFAULT 1,
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
);

CREATE INDEX idx_nodes_domain_layer ON nodes(domain, layer);
CREATE INDEX idx_nodes_parent       ON nodes(parent_id);

CREATE TABLE edges (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  source_id   TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  target_id   TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  type        TEXT NOT NULL,
  properties  TEXT NOT NULL DEFAULT '{}',
  revision    INTEGER NOT NULL DEFAULT 1,
  created_at  INTEGER NOT NULL,
  UNIQUE(source_id, target_id, type)
);

CREATE INDEX idx_edges_source ON edges(source_id, type);
CREATE INDEX idx_edges_target ON edges(target_id, type);

CREATE TABLE changes (
  seq         INTEGER PRIMARY KEY AUTOINCREMENT,
  entity      TEXT NOT NULL,
  entity_id   TEXT NOT NULL,
  op          TEXT NOT NULL,
  revision    INTEGER,
  at          INTEGER NOT NULL
);

CREATE INDEX idx_changes_seq    ON changes(seq);
CREATE INDEX idx_changes_entity ON changes(entity, entity_id);

-- +goose Down
DROP INDEX IF EXISTS idx_changes_entity;
DROP INDEX IF EXISTS idx_changes_seq;
DROP TABLE IF EXISTS changes;
DROP INDEX IF EXISTS idx_edges_target;
DROP INDEX IF EXISTS idx_edges_source;
DROP TABLE IF EXISTS edges;
DROP INDEX IF EXISTS idx_nodes_parent;
DROP INDEX IF EXISTS idx_nodes_domain_layer;
DROP TABLE IF EXISTS nodes;
DROP TABLE IF EXISTS domains;
