-- +goose Up
INSERT INTO sys_dept (id, parent_id, dept_name, dept_code, sort_order, status, is_builtin, remark)
VALUES (1, 0, '总部', 'ROOT', 1, 1, 1, '内置根部门');

INSERT INTO sys_role (id, role_name, role_code, status, sort_order, is_builtin, remark)
VALUES (1, '超级管理员', 'ADMIN', 1, 1, 1, '内置超级管理员角色');

-- Initial credentials: admin / admin123. Local development only.
INSERT INTO sys_user (id, username, nickname, password, gender, dept_id, status, is_builtin, remark)
VALUES (1, 'admin', '管理员', '$2a$10$H/LRi5UDBbOtTBHaABVki.qtv6zGtB8FjdvEbp7ahvSXBzYSTXpi6', 'UNSPECIFIED', 1, 1, 1, '内置管理员用户');

INSERT INTO sys_user_role (id, user_id, role_id) VALUES (1, 1, 1);

INSERT INTO sys_menu (id, parent_id, menu_name, menu_type, path, component, icon, permission_code, sort_order, visible, status, is_builtin)
VALUES
    (1, 0, '系统管理', 'DIR', '/system', NULL, 'Settings', 'system', 1, 1, 1, 1),
    (2, 1, '用户管理', 'MENU', '/system/user', 'system/user/index', 'User', 'system:user', 1, 1, 1, 1),
    (3, 1, '角色管理', 'MENU', '/system/role', 'system/role/index', 'Shield', 'system:role', 2, 1, 1, 1),
    (4, 1, '菜单管理', 'MENU', '/system/menu', 'system/menu/index', 'Menu', 'system:menu', 3, 1, 1, 1),
    (5, 1, '部门管理', 'MENU', '/system/dept', 'system/dept/index', 'Building2', 'system:dept', 4, 1, 1, 1),
    (6, 1, '字典管理', 'MENU', '/system/dict', 'system/dict/index', 'BookOpen', 'system:dict', 5, 1, 1, 1),
    (7, 0, '日志管理', 'DIR', '/log', NULL, 'ListChecks', 'system:log', 2, 1, 1, 1),
    (8, 7, '登录日志', 'MENU', '/system/login-log', 'system/login-log/index', 'LogIn', 'system:login-log', 1, 1, 1, 1),
    (9, 7, '操作日志', 'MENU', '/system/oper-log', 'system/oper-log/index', 'ClipboardList', 'system:oper-log', 2, 1, 1, 1),
    (10, 0, '文件管理', 'MENU', '/system/file', 'system/file/index', 'File', 'system:file', 3, 1, 1, 1),
    (11, 1, '配置管理', 'MENU', '/system/config', 'system/config/index', 'Settings2', 'system:config', 6, 1, 1, 1);

INSERT INTO sys_role_menu (id, role_id, menu_id)
SELECT id, 1, id FROM sys_menu WHERE id BETWEEN 1 AND 11 ORDER BY id;

SELECT setval(pg_get_serial_sequence('sys_dept', 'id'), (SELECT MAX(id) FROM sys_dept), true);
SELECT setval(pg_get_serial_sequence('sys_role', 'id'), (SELECT MAX(id) FROM sys_role), true);
SELECT setval(pg_get_serial_sequence('sys_user', 'id'), (SELECT MAX(id) FROM sys_user), true);
SELECT setval(pg_get_serial_sequence('sys_user_role', 'id'), (SELECT MAX(id) FROM sys_user_role), true);
SELECT setval(pg_get_serial_sequence('sys_menu', 'id'), (SELECT MAX(id) FROM sys_menu), true);
SELECT setval(pg_get_serial_sequence('sys_role_menu', 'id'), (SELECT MAX(id) FROM sys_role_menu), true);

-- +goose Down
DELETE FROM sys_role_menu WHERE id BETWEEN 1 AND 11 AND role_id = 1 AND menu_id BETWEEN 1 AND 11;
DELETE FROM sys_user_role WHERE id = 1 AND user_id = 1 AND role_id = 1;
DELETE FROM sys_menu WHERE id BETWEEN 1 AND 11 AND is_builtin = 1;
DELETE FROM sys_user WHERE id = 1 AND username = 'admin' AND is_builtin = 1;
DELETE FROM sys_role WHERE id = 1 AND role_code = 'ADMIN' AND is_builtin = 1;
DELETE FROM sys_dept WHERE id = 1 AND dept_code = 'ROOT' AND is_builtin = 1;
