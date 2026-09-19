-- +goose Up
-- Start from a closed state. cmd/migrate applies the environment default once
-- after this corrective migration has been recorded.
UPDATE sys_config
SET config_value = 'false',
    update_time = CURRENT_TIMESTAMP
WHERE config_key = 'system.log-clear-enabled'
  AND is_builtin = 1
  AND deleted = 0;

-- +goose Down
UPDATE sys_config
SET config_value = 'true',
    update_time = CURRENT_TIMESTAMP
WHERE config_key = 'system.log-clear-enabled'
  AND is_builtin = 1
  AND deleted = 0;
