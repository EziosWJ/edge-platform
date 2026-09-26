-- SQLite migration is retained for local/test compatibility only.
-- +goose Up
INSERT INTO sys_menu (id, parent_id, menu_name, menu_type, path, permission_code, sort_order, visible, status, is_builtin)
VALUES
    (15, 0, 'HMI 页面列表', 'MENU', '/hmi', 'hmi:list', 1, 0, 1, 1),
    (16, 0, 'HMI 页面详情', 'MENU', '/hmi', 'hmi:detail', 2, 0, 1, 1),
    (17, 0, 'HMI 页面编辑', 'MENU', '/hmi', 'hmi:edit', 3, 0, 1, 1),
    (18, 0, 'HMI 页面发布', 'MENU', '/hmi', 'hmi:publish', 4, 0, 1, 1),
    (19, 0, 'HMI 页面运行', 'MENU', '/hmi', 'hmi:run', 5, 0, 1, 1);
INSERT INTO sys_role_menu (id, role_id, menu_id)
VALUES (15, 1, 15), (16, 1, 16), (17, 1, 17), (18, 1, 18), (19, 1, 19);

-- +goose Down
DELETE FROM sys_role_menu WHERE id BETWEEN 15 AND 19 AND role_id = 1 AND menu_id BETWEEN 15 AND 19;
DELETE FROM sys_menu WHERE id BETWEEN 15 AND 19 AND permission_code IN ('hmi:list','hmi:detail','hmi:edit','hmi:publish','hmi:run') AND is_builtin = 1;
