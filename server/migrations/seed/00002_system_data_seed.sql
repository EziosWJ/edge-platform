-- +goose Up
INSERT INTO sys_dict_type (id, dict_name, dict_code, status, sort_order, is_builtin, remark)
VALUES
    (1, '用户状态', 'USER_STATUS', 1, 1, 1, '用户启用状态'),
    (2, '性别', 'GENDER', 1, 2, 1, '用户性别'),
    (3, '菜单类型', 'MENU_TYPE', 1, 3, 1, '菜单类型'),
    (4, '通用状态', 'COMMON_STATUS', 1, 4, 1, '通用启用状态'),
    (5, '操作类型', 'OPERATION_TYPE', 1, 5, 1, '操作日志类型'),
    (6, '登录状态', 'LOGIN_STATUS', 1, 6, 1, '登录日志状态'),
    (7, '文件业务模块', 'FILE_BUSINESS_MODULE', 1, 7, 1, '文件业务归属'),
    (8, '配置类型', 'CONFIG_TYPE', 1, 8, 1, '系统配置归属类型'),
    (9, '配置值类型', 'CONFIG_VALUE_TYPE', 1, 9, 1, '配置值数据类型');

INSERT INTO sys_dict_data (id, dict_type_id, dict_label, dict_value, sort_order, remark)
VALUES
    (1, 1, '启用', '1', 1, NULL),
    (2, 1, '禁用', '0', 2, NULL),
    (3, 2, '未知', 'UNSPECIFIED', 1, NULL),
    (4, 2, '男', 'MALE', 2, NULL),
    (5, 2, '女', 'FEMALE', 3, NULL),
    (6, 3, '目录', 'DIR', 1, NULL),
    (7, 3, '菜单', 'MENU', 2, NULL),
    (8, 3, '外链', 'LINK', 3, NULL),
    (9, 4, '启用', '1', 1, NULL),
    (10, 4, '禁用', '0', 2, NULL),
    (11, 5, '新增', 'CREATE', 1, NULL),
    (12, 5, '修改', 'UPDATE', 2, NULL),
    (13, 5, '删除', 'DELETE', 3, NULL),
    (14, 5, '导入', 'IMPORT', 4, NULL),
    (15, 5, '导出', 'EXPORT', 5, NULL),
    (16, 6, '成功', 'SUCCESS', 1, NULL),
    (17, 6, '失败', 'FAIL', 2, NULL),
    (18, 7, '头像', 'avatar', 1, NULL),
    (19, 7, '导入', 'import', 2, NULL),
    (20, 7, '附件', 'attachment', 3, NULL),
    (21, 8, '系统配置', 'SYSTEM', 1, NULL),
    (22, 8, '自定义配置', 'CUSTOM', 2, NULL),
    (23, 9, '文本', 'TEXT', 1, NULL),
    (24, 9, '数字', 'NUMBER', 2, NULL),
    (25, 9, '布尔', 'BOOLEAN', 3, NULL);

INSERT INTO sys_config (id, config_name, config_key, config_value, config_type, value_type, status, is_builtin, remark)
VALUES
    (1, '日志清空开关', 'system.log-clear-enabled', 'true', 'SYSTEM', 'BOOLEAN', 1, 1,
     '控制日志清空接口是否可用，dev 默认开启，prod 默认关闭');

SELECT setval(pg_get_serial_sequence('sys_dict_type', 'id'), (SELECT MAX(id) FROM sys_dict_type), true);
SELECT setval(pg_get_serial_sequence('sys_dict_data', 'id'), (SELECT MAX(id) FROM sys_dict_data), true);
SELECT setval(pg_get_serial_sequence('sys_config', 'id'), (SELECT MAX(id) FROM sys_config), true);

-- +goose Down
DELETE FROM sys_config
WHERE id = 1 AND config_key = 'system.log-clear-enabled' AND is_builtin = 1;

DELETE FROM sys_dict_data WHERE id BETWEEN 1 AND 25;
DELETE FROM sys_dict_type WHERE id BETWEEN 1 AND 9 AND is_builtin = 1;

SELECT setval(pg_get_serial_sequence('sys_dict_type', 'id'), COALESCE((SELECT MAX(id) FROM sys_dict_type), 1), EXISTS (SELECT 1 FROM sys_dict_type));
SELECT setval(pg_get_serial_sequence('sys_dict_data', 'id'), COALESCE((SELECT MAX(id) FROM sys_dict_data), 1), EXISTS (SELECT 1 FROM sys_dict_data));
SELECT setval(pg_get_serial_sequence('sys_config', 'id'), COALESCE((SELECT MAX(id) FROM sys_config), 1), EXISTS (SELECT 1 FROM sys_config));
