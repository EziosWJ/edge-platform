-- SQLite migration is retained for local/test compatibility only.
-- +goose Up
INSERT INTO sys_menu (id, parent_id, menu_name, menu_type, path, permission_code, sort_order, visible, status, is_builtin)
VALUES
    (13, 0, 'Command 查询', 'MENU', '/command', 'command:list', 2, 0, 1, 1),
    (14, 0, 'Command 详情', 'MENU', '/command', 'command:detail', 3, 0, 1, 1);

INSERT INTO sys_role_menu (id, role_id, menu_id)
VALUES
    (13, 1, 13),
    (14, 1, 14);

-- +goose Down
DELETE FROM sys_role_menu WHERE id IN (13, 14) AND role_id = 1 AND menu_id IN (13, 14);
DELETE FROM sys_menu WHERE id IN (13, 14) AND permission_code IN ('command:list', 'command:detail') AND is_builtin = 1;
