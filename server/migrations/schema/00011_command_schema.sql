-- +goose Up
CREATE TABLE command (
    command_id UUID NOT NULL,
    device_id TEXT NOT NULL,
    edge_id TEXT NOT NULL,
    source_device_id TEXT NOT NULL,
    topic TEXT NOT NULL,
    name VARCHAR(256) NOT NULL,
    args JSONB NOT NULL,
    requested_by BIGINT NOT NULL,
    request_hash VARCHAR(64) NOT NULL,
    issued_at TIMESTAMP WITH TIME ZONE NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'PENDING',
    delivery_expired_at TIMESTAMP WITH TIME ZONE,
    edge_received_at TIMESTAMP WITH TIME ZONE,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    result_received_at TIMESTAMP WITH TIME ZONE,
    result JSONB,
    error_type VARCHAR(100),
    error_message TEXT,
    CONSTRAINT pk_command PRIMARY KEY (command_id),
    CONSTRAINT fk_command_device FOREIGN KEY (device_id) REFERENCES device (device_id) ON DELETE RESTRICT,
    CONSTRAINT fk_command_requester FOREIGN KEY (requested_by) REFERENCES sys_user (id) ON DELETE RESTRICT,
    CONSTRAINT ck_command_name_not_blank CHECK (length(trim(name)) > 0),
    CONSTRAINT ck_command_ids_not_blank CHECK (length(edge_id) > 0 AND length(source_device_id) > 0 AND length(topic) > 0),
    CONSTRAINT ck_command_hash_not_blank CHECK (length(request_hash) = 64),
    CONSTRAINT ck_command_status CHECK (status IN ('PENDING', 'ACCEPTED', 'REJECTED', 'EXPIRED', 'SUCCEEDED', 'FAILED')),
    CONSTRAINT ck_command_expiry_after_issue CHECK (expires_at > issued_at)
);
CREATE INDEX idx_command_requested_by_issued_at ON command (requested_by, issued_at DESC);
CREATE INDEX idx_command_device_issued_at ON command (device_id, issued_at DESC);

CREATE TABLE command_delivery (
    command_id UUID NOT NULL,
    topic TEXT NOT NULL,
    payload BYTEA NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    last_attempt_at TIMESTAMP WITH TIME ZONE,
    last_error TEXT,
    next_attempt_at TIMESTAMP WITH TIME ZONE,
    CONSTRAINT pk_command_delivery PRIMARY KEY (command_id),
    CONSTRAINT fk_command_delivery_command FOREIGN KEY (command_id) REFERENCES command (command_id) ON DELETE CASCADE,
    CONSTRAINT ck_command_delivery_topic_not_blank CHECK (length(topic) > 0),
    CONSTRAINT ck_command_delivery_attempt_count CHECK (attempt_count >= 0)
);

-- +goose Down
DROP TABLE IF EXISTS command_delivery;
DROP TABLE IF EXISTS command;
