-- SQLite has no independent identity sequence to advance.
-- +goose Up
SELECT 1;

-- +goose Down
SELECT 1;
