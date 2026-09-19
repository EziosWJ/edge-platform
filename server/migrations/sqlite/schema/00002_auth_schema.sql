-- +goose Up
CREATE TABLE sys_dept (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    parent_id INTEGER NOT NULL DEFAULT 0,
    dept_name VARCHAR(100) NOT NULL,
    dept_code VARCHAR(50) NOT NULL,
    leader VARCHAR(50),
    phone VARCHAR(20),
    email VARCHAR(100),
    sort_order INTEGER NOT NULL DEFAULT 0,
    status INTEGER NOT NULL DEFAULT 1,
    is_builtin INTEGER NOT NULL DEFAULT 0,
    remark VARCHAR(500),
    create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    create_by INTEGER,
    update_by INTEGER,
    deleted INTEGER NOT NULL DEFAULT 0,
    CONSTRAINT uk_sys_dept_code UNIQUE (dept_code),
    CONSTRAINT ck_sys_dept_parent_id CHECK (parent_id >= 0),
    CONSTRAINT ck_sys_dept_status CHECK (status IN (0, 1)),
    CONSTRAINT ck_sys_dept_is_builtin CHECK (is_builtin IN (0, 1)),
    CONSTRAINT ck_sys_dept_deleted CHECK (deleted IN (0, 1))
);

CREATE INDEX idx_sys_dept_parent_id ON sys_dept (parent_id);
CREATE INDEX idx_sys_dept_deleted_status_sort ON sys_dept (deleted, status, sort_order);

CREATE TABLE sys_user (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username VARCHAR(50) NOT NULL,
    nickname VARCHAR(50) NOT NULL,
    password VARCHAR(100) NOT NULL,
    phone VARCHAR(20),
    email VARCHAR(100),
    avatar VARCHAR(255),
    gender VARCHAR(20) NOT NULL DEFAULT 'UNSPECIFIED',
    dept_id INTEGER,
    status INTEGER NOT NULL DEFAULT 1,
    is_builtin INTEGER NOT NULL DEFAULT 0,
    last_login_time DATETIME,
    last_login_ip VARCHAR(45),
    remark VARCHAR(500),
    create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    create_by INTEGER,
    update_by INTEGER,
    deleted INTEGER NOT NULL DEFAULT 0,
    CONSTRAINT uk_sys_user_username UNIQUE (username),
    CONSTRAINT fk_sys_user_dept FOREIGN KEY (dept_id) REFERENCES sys_dept (id) ON DELETE SET NULL,
    CONSTRAINT ck_sys_user_status CHECK (status IN (0, 1)),
    CONSTRAINT ck_sys_user_is_builtin CHECK (is_builtin IN (0, 1)),
    CONSTRAINT ck_sys_user_deleted CHECK (deleted IN (0, 1))
);

CREATE INDEX idx_sys_user_dept_id ON sys_user (dept_id);
CREATE INDEX idx_sys_user_deleted_status ON sys_user (deleted, status);

CREATE TABLE sys_role (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    role_name VARCHAR(50) NOT NULL,
    role_code VARCHAR(50) NOT NULL,
    status INTEGER NOT NULL DEFAULT 1,
    sort_order INTEGER NOT NULL DEFAULT 0,
    is_builtin INTEGER NOT NULL DEFAULT 0,
    remark VARCHAR(500),
    create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    create_by INTEGER,
    update_by INTEGER,
    deleted INTEGER NOT NULL DEFAULT 0,
    CONSTRAINT uk_sys_role_code UNIQUE (role_code),
    CONSTRAINT ck_sys_role_status CHECK (status IN (0, 1)),
    CONSTRAINT ck_sys_role_is_builtin CHECK (is_builtin IN (0, 1)),
    CONSTRAINT ck_sys_role_deleted CHECK (deleted IN (0, 1))
);

CREATE INDEX idx_sys_role_deleted_status_sort ON sys_role (deleted, status, sort_order);

CREATE TABLE sys_menu (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    parent_id INTEGER NOT NULL DEFAULT 0,
    menu_name VARCHAR(50) NOT NULL,
    menu_type VARCHAR(20) NOT NULL,
    path VARCHAR(255),
    component VARCHAR(255),
    external_url VARCHAR(500),
    icon VARCHAR(100),
    permission_code VARCHAR(100),
    sort_order INTEGER NOT NULL DEFAULT 0,
    visible INTEGER NOT NULL DEFAULT 1,
    status INTEGER NOT NULL DEFAULT 1,
    is_builtin INTEGER NOT NULL DEFAULT 0,
    remark VARCHAR(500),
    create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    update_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    create_by INTEGER,
    update_by INTEGER,
    deleted INTEGER NOT NULL DEFAULT 0,
    CONSTRAINT uk_sys_menu_permission_code UNIQUE (permission_code),
    CONSTRAINT ck_sys_menu_parent_id CHECK (parent_id >= 0),
    CONSTRAINT ck_sys_menu_type CHECK (menu_type IN ('DIR', 'MENU', 'LINK')),
    CONSTRAINT ck_sys_menu_visible CHECK (visible IN (0, 1)),
    CONSTRAINT ck_sys_menu_status CHECK (status IN (0, 1)),
    CONSTRAINT ck_sys_menu_is_builtin CHECK (is_builtin IN (0, 1)),
    CONSTRAINT ck_sys_menu_deleted CHECK (deleted IN (0, 1))
);

CREATE INDEX idx_sys_menu_parent_id ON sys_menu (parent_id);
CREATE INDEX idx_sys_menu_deleted_status_sort ON sys_menu (deleted, status, sort_order);
CREATE INDEX idx_sys_menu_type ON sys_menu (menu_type);

CREATE TABLE sys_user_role (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    role_id INTEGER NOT NULL,
    create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    create_by INTEGER,
    CONSTRAINT uk_sys_user_role UNIQUE (user_id, role_id),
    CONSTRAINT fk_sys_user_role_user FOREIGN KEY (user_id) REFERENCES sys_user (id) ON DELETE CASCADE,
    CONSTRAINT fk_sys_user_role_role FOREIGN KEY (role_id) REFERENCES sys_role (id) ON DELETE CASCADE
);

CREATE INDEX idx_sys_user_role_user_id ON sys_user_role (user_id);
CREATE INDEX idx_sys_user_role_role_id ON sys_user_role (role_id);

CREATE TABLE sys_role_menu (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    role_id INTEGER NOT NULL,
    menu_id INTEGER NOT NULL,
    create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    create_by INTEGER,
    CONSTRAINT uk_sys_role_menu UNIQUE (role_id, menu_id),
    CONSTRAINT fk_sys_role_menu_role FOREIGN KEY (role_id) REFERENCES sys_role (id) ON DELETE CASCADE,
    CONSTRAINT fk_sys_role_menu_menu FOREIGN KEY (menu_id) REFERENCES sys_menu (id) ON DELETE CASCADE
);

CREATE INDEX idx_sys_role_menu_role_id ON sys_role_menu (role_id);
CREATE INDEX idx_sys_role_menu_menu_id ON sys_role_menu (menu_id);

CREATE TABLE sys_login_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username VARCHAR(50) NOT NULL,
    login_status VARCHAR(20) NOT NULL,
    login_ip VARCHAR(45),
    login_location VARCHAR(100),
    browser VARCHAR(100),
    os VARCHAR(100),
    user_agent VARCHAR(500),
    message VARCHAR(500),
    login_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT ck_sys_login_log_status CHECK (login_status IN ('SUCCESS', 'FAIL'))
);

CREATE INDEX idx_sys_login_log_username ON sys_login_log (username);
CREATE INDEX idx_sys_login_log_status ON sys_login_log (login_status);
CREATE INDEX idx_sys_login_log_time ON sys_login_log (login_time);

CREATE TABLE auth_session (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    jti VARCHAR(64) NOT NULL,
    expires_at DATETIME NOT NULL,
    revoked_at DATETIME,
    create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_auth_session_jti UNIQUE (jti),
    CONSTRAINT fk_auth_session_user FOREIGN KEY (user_id) REFERENCES sys_user (id) ON DELETE CASCADE,
    CONSTRAINT ck_auth_session_jti_not_blank CHECK (length(trim(jti)) > 0)
);

CREATE INDEX idx_auth_session_user_active ON auth_session (user_id, revoked_at, expires_at);
CREATE INDEX idx_auth_session_expires_at ON auth_session (expires_at);

-- +goose Down
DROP TABLE IF EXISTS auth_session;
DROP TABLE IF EXISTS sys_login_log;
DROP TABLE IF EXISTS sys_role_menu;
DROP TABLE IF EXISTS sys_user_role;
DROP TABLE IF EXISTS sys_menu;
DROP TABLE IF EXISTS sys_role;
DROP TABLE IF EXISTS sys_user;
DROP TABLE IF EXISTS sys_dept;
