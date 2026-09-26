-- +goose Up
-- Command permission seeds use explicit IDs. Keep the identity sequences ahead
-- of both seeded rows and any IDs already allocated on existing databases.
SELECT setval(
    pg_get_serial_sequence('sys_menu', 'id'),
    GREATEST((SELECT MAX(id) FROM sys_menu), (SELECT last_value FROM sys_menu_id_seq)),
    true
);

SELECT setval(
    pg_get_serial_sequence('sys_role_menu', 'id'),
    GREATEST((SELECT MAX(id) FROM sys_role_menu), (SELECT last_value FROM sys_role_menu_id_seq)),
    true
);

-- +goose Down
-- Sequence positions must not move backwards when rolling back this migration.
SELECT 1;
