-- +goose Up
ALTER TABLE sys_file DROP CONSTRAINT IF EXISTS ck_sys_file_md5;

-- +goose Down
ALTER TABLE sys_file
    ADD CONSTRAINT ck_sys_file_md5 CHECK (file_md5 IS NULL OR file_md5 ~ '^[0-9A-Fa-f]{32}$');
