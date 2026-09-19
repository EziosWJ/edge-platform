-- +goose Up
CREATE TABLE sys_file (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    original_name VARCHAR(255) NOT NULL,
    storage_name VARCHAR(255) NOT NULL,
    extension VARCHAR(50),
    mime_type VARCHAR(100),
    file_size INTEGER NOT NULL DEFAULT 0,
    file_md5 VARCHAR(32),
    storage_path VARCHAR(500) NOT NULL,
    access_url VARCHAR(500),
    business_module VARCHAR(50),
    status INTEGER NOT NULL DEFAULT 1,
    remark VARCHAR(500),
    create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    create_by INTEGER,
    update_by INTEGER,
    deleted INTEGER NOT NULL DEFAULT 0,
    CONSTRAINT uk_sys_file_storage_path UNIQUE (storage_path),
    CONSTRAINT ck_sys_file_original_name_not_blank CHECK (length(trim(original_name)) > 0),
    CONSTRAINT ck_sys_file_storage_name_not_blank CHECK (length(trim(storage_name)) > 0),
    CONSTRAINT ck_sys_file_storage_path_not_blank CHECK (length(trim(storage_path)) > 0),
    CONSTRAINT ck_sys_file_size CHECK (file_size BETWEEN 0 AND 52428800),
    CONSTRAINT ck_sys_file_md5_length CHECK (file_md5 IS NULL OR length(file_md5) = 32),
    CONSTRAINT ck_sys_file_status CHECK (status IN (0, 1)),
    CONSTRAINT ck_sys_file_deleted CHECK (deleted IN (0, 1))
);

CREATE INDEX idx_sys_file_md5 ON sys_file (file_md5);
CREATE INDEX idx_sys_file_deleted_created ON sys_file (deleted, create_time DESC, id DESC);
CREATE INDEX idx_sys_file_deleted_status_created ON sys_file (deleted, status, create_time DESC, id DESC);
CREATE INDEX idx_sys_file_deleted_business_created ON sys_file (deleted, business_module, create_time DESC, id DESC);

-- +goose Down
DROP TABLE IF EXISTS sys_file;
