-- +goose Up
CREATE TABLE data_point (
    data_point_id UUID NOT NULL,
    device_id TEXT NOT NULL,
    point_key VARCHAR(64) NOT NULL,
    name VARCHAR(200) NOT NULL,
    value_type VARCHAR(10) NOT NULL,
    unit VARCHAR(100),
    precision SMALLINT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    config_generation BIGINT NOT NULL DEFAULT 1,
    effective_at TIMESTAMP WITH TIME ZONE NOT NULL,
    CONSTRAINT pk_data_point PRIMARY KEY (data_point_id),
    CONSTRAINT fk_data_point_device FOREIGN KEY (device_id) REFERENCES device (device_id) ON DELETE RESTRICT,
    CONSTRAINT uq_data_point_device_point_key UNIQUE (device_id, point_key),
    CONSTRAINT ck_data_point_point_key CHECK (point_key ~ '^[a-z][a-z0-9_]{0,63}$'),
    CONSTRAINT ck_data_point_value_type CHECK (value_type IN ('NUMBER', 'BOOLEAN')),
    CONSTRAINT ck_data_point_precision CHECK (precision IS NULL OR (precision >= 0 AND precision <= 12)),
    CONSTRAINT ck_data_point_generation CHECK (config_generation > 0)
);
CREATE INDEX idx_data_point_device_point_key ON data_point (device_id ASC, point_key ASC, data_point_id ASC);

CREATE TABLE source_mapping (
    data_point_id UUID NOT NULL,
    source_type VARCHAR(30) NOT NULL,
    function_code SMALLINT NOT NULL,
    address INTEGER NOT NULL,
    encoding VARCHAR(20) NOT NULL,
    word_order VARCHAR(20),
    byte_order VARCHAR(20) NOT NULL,
    bit_index SMALLINT,
    scale DOUBLE PRECISION NOT NULL,
    "offset" DOUBLE PRECISION NOT NULL,
    CONSTRAINT pk_source_mapping PRIMARY KEY (data_point_id),
    CONSTRAINT fk_source_mapping_data_point FOREIGN KEY (data_point_id) REFERENCES data_point (data_point_id) ON DELETE CASCADE,
    CONSTRAINT ck_source_mapping_type CHECK (source_type = 'MODBUS_REGISTER'),
    CONSTRAINT ck_source_mapping_function CHECK (function_code IN (3, 4)),
    CONSTRAINT ck_source_mapping_address CHECK (address >= 0 AND address <= 65535),
    CONSTRAINT ck_source_mapping_encoding CHECK (encoding IN ('UINT16', 'INT16', 'UINT32', 'INT32', 'FLOAT32', 'BOOLEAN_BIT')),
    CONSTRAINT ck_source_mapping_byte_order CHECK (byte_order IN ('BIG_ENDIAN', 'LITTLE_ENDIAN')),
    CONSTRAINT ck_source_mapping_bit CHECK (bit_index IS NULL OR (bit_index >= 0 AND bit_index <= 15)),
    CONSTRAINT ck_source_mapping_finite CHECK (scale <> 'NaN'::double precision AND "offset" <> 'NaN'::double precision AND scale <> 'Infinity'::double precision AND scale <> '-Infinity'::double precision AND "offset" <> 'Infinity'::double precision AND "offset" <> '-Infinity'::double precision)
);

CREATE TABLE current_value (
    data_point_id UUID NOT NULL,
    value_type VARCHAR(10) NOT NULL,
    number_value DOUBLE PRECISION,
    boolean_value BOOLEAN,
    quality VARCHAR(10) NOT NULL,
    source_timestamp TIMESTAMP WITH TIME ZONE,
    observed_at TIMESTAMP WITH TIME ZONE,
    revision BIGINT NOT NULL DEFAULT 0,
    CONSTRAINT pk_current_value PRIMARY KEY (data_point_id),
    CONSTRAINT fk_current_value_data_point FOREIGN KEY (data_point_id) REFERENCES data_point (data_point_id) ON DELETE CASCADE,
    CONSTRAINT ck_current_value_type CHECK (value_type IN ('NUMBER', 'BOOLEAN')),
    CONSTRAINT ck_current_value_quality CHECK (quality IN ('NO_DATA', 'GOOD', 'BAD')),
    CONSTRAINT ck_current_value_revision CHECK (revision >= 0),
    CONSTRAINT ck_current_value_typed CHECK ((value_type = 'NUMBER' AND boolean_value IS NULL) OR (value_type = 'BOOLEAN' AND number_value IS NULL)),
    CONSTRAINT ck_current_value_no_data CHECK (quality <> 'NO_DATA' OR (number_value IS NULL AND boolean_value IS NULL AND source_timestamp IS NULL AND observed_at IS NULL))
);

-- +goose Down
DROP TABLE IF EXISTS current_value;
DROP TABLE IF EXISTS source_mapping;
DROP TABLE IF EXISTS data_point;
