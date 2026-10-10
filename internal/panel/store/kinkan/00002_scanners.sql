-- +goose Up
CREATE TABLE kinkan_scanners (
 node_id BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
 epoch TEXT NOT NULL,
 record_key TEXT NOT NULL,
 day BIGINT NOT NULL,
 count BIGINT NOT NULL CHECK (count > 0),
 payload JSONB NOT NULL,
 PRIMARY KEY (node_id, epoch, record_key)
);
CREATE INDEX kinkan_scanners_day ON kinkan_scanners(node_id, day DESC);

-- +goose Down
DROP TABLE kinkan_scanners;
