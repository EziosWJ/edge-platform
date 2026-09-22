-- +goose Up
INSERT INTO sys_menu (id, parent_id, menu_name, menu_type, path, permission_code, sort_order, visible, status, is_builtin)
VALUES (12, 0, 'Command 执行', 'MENU', '/command', 'command:execute', 1, 0, 1, 1);

INSERT INTO sys_role_menu (id, role_id, menu_id) VALUES (12, 1, 12);

-- +goose Down
DELETE FROM sys_role_menu WHERE id = 12 AND role_id = 1 AND menu_id = 12;
DELETE FROM sys_menu WHERE id = 12 AND permission_code = 'command:execute' AND is_builtin = 1;
