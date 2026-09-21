-- +goose Up
CREATE TABLE edge (
    edge_id TEXT NOT NULL,
    status TEXT NOT NULL,
    registered_at DATETIME NOT NULL,
    last_seen_at DATETIME NOT NULL,
    CONSTRAINT pk_edge PRIMARY KEY (edge_id),
    CONSTRAINT ck_edge_id_not_blank CHECK (length(edge_id) > 0),
    CONSTRAINT ck_edge_status CHECK (status IN ('ONLINE', 'OFFLINE'))
);

CREATE INDEX idx_edge_registered_at_edge_id ON edge (registered_at DESC, edge_id ASC);

-- +goose Down
DROP TABLE IF EXISTS edge;
