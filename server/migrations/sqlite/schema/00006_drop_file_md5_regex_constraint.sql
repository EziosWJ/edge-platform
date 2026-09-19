-- +goose Up
-- PostgreSQL removes its regex-only constraint at this logical version.
-- SQLite never uses that operator; the portable length check and the Go
-- storage boundary retain the checksum invariant.
SELECT 1;

-- +goose Down
SELECT 1;
