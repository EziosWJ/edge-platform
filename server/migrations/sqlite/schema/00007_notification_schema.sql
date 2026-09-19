-- +goose Up
CREATE TABLE sys_notification (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title VARCHAR(200) NOT NULL,
    content TEXT NOT NULL,
    source_type VARCHAR(30) NOT NULL DEFAULT 'MANUAL',
    publisher_id INTEGER,
    publish_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT ck_sys_notification_title_not_blank CHECK (length(trim(title)) > 0),
    CONSTRAINT ck_sys_notification_content_not_blank CHECK (length(trim(content)) > 0),
    CONSTRAINT ck_sys_notification_source CHECK (source_type IN ('MANUAL', 'ROLE_CHANGE'))
);

CREATE TABLE sys_notification_recipient (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    notification_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    is_read INTEGER NOT NULL DEFAULT 0,
    read_time DATETIME,
    create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_sys_notification_recipient UNIQUE (notification_id, user_id),
    CONSTRAINT fk_notification_recipient_notification FOREIGN KEY (notification_id) REFERENCES sys_notification (id) ON DELETE CASCADE,
    CONSTRAINT fk_notification_recipient_user FOREIGN KEY (user_id) REFERENCES sys_user (id) ON DELETE RESTRICT,
    CONSTRAINT ck_sys_notification_recipient_read CHECK (is_read IN (0, 1))
);

CREATE INDEX idx_sys_notification_publish_time ON sys_notification (publish_time DESC, id DESC);
CREATE INDEX idx_sys_notification_recipient_user_read ON sys_notification_recipient (user_id, is_read, notification_id DESC);

-- +goose Down
DROP TABLE IF EXISTS sys_notification_recipient;
DROP TABLE IF EXISTS sys_notification;
