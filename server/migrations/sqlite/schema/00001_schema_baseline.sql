-- +goose Up
-- Keep the SQLite stream versioned from the same logical baseline as PostgreSQL.
SELECT 1;

-- +goose Down
SELECT 1;
