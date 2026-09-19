-- +goose Up
-- This no-op baseline proves that a new database can enter the schema migration
-- stream before business tables are introduced by their owning feature issues.
SELECT 1;

-- +goose Down
SELECT 1;
