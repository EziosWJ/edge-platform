-- +goose Up
CREATE TABLE device (
    device_id TEXT NOT NULL,
    edge_id TEXT NOT NULL,
    source_device_id TEXT NOT NULL,
    communication_status TEXT NOT NULL,
    registered_at DATETIME NOT NULL,
    last_seen_at DATETIME NOT NULL,
    last_attempt_at DATETIME,
    last_success_at DATETIME,
    communication_error TEXT,
    CONSTRAINT pk_device PRIMARY KEY (device_id),
    CONSTRAINT fk_device_edge FOREIGN KEY (edge_id) REFERENCES edge (edge_id) ON DELETE RESTRICT,
    CONSTRAINT uq_device_edge_source UNIQUE (edge_id, source_device_id),
    CONSTRAINT ck_device_id_not_blank CHECK (length(device_id) > 0),
    CONSTRAINT ck_device_edge_id_not_blank CHECK (length(edge_id) > 0),
    CONSTRAINT ck_device_source_id_not_blank CHECK (length(source_device_id) > 0),
    CONSTRAINT ck_device_status CHECK (communication_status IN ('INITIAL', 'ONLINE', 'DEGRADED', 'OFFLINE'))
);

CREATE INDEX idx_device_registered_at_device_id ON device (registered_at DESC, device_id ASC);
CREATE INDEX idx_device_edge_id ON device (edge_id);
CREATE INDEX idx_device_source_device_id ON device (source_device_id);
CREATE INDEX idx_device_communication_status ON device (communication_status);

-- +goose Down
DROP TABLE IF EXISTS device;
