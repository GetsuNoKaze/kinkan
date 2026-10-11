-- +goose Up
CREATE TABLE kinkan_probe_runs (
 id BIGSERIAL PRIMARY KEY,
 node_id BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
 started_at BIGINT NOT NULL,
 inbound_id BIGINT NOT NULL DEFAULT 0,
 verdict TEXT NOT NULL,
 inbounds INTEGER NOT NULL,
 report JSONB NOT NULL
);
CREATE INDEX kinkan_probe_runs_node ON kinkan_probe_runs(node_id, started_at DESC);

-- +goose Down
DROP TABLE kinkan_probe_runs;
