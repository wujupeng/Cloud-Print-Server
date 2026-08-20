-- 0001_init.down.sql: 反向迁移

DROP INDEX IF EXISTS idx_config_dispatches_agent_ver;
DROP INDEX IF EXISTS idx_audit_logs_action_ts;
DROP INDEX IF EXISTS idx_audit_logs_user_ts;
DROP INDEX IF EXISTS idx_agent_logs_ts;
DROP INDEX IF EXISTS idx_agent_logs_agent_ts;
DROP INDEX IF EXISTS idx_documents_cleanup;
DROP INDEX IF EXISTS idx_tasks_agent_status;
DROP INDEX IF EXISTS idx_tasks_status_retry;
DROP INDEX IF EXISTS idx_tasks_user_submitted;
DROP INDEX IF EXISTS idx_devices_agent_status;
DROP INDEX IF EXISTS idx_agents_factory_id;
DROP INDEX IF EXISTS idx_users_username;

DROP TABLE IF EXISTS config_meta;
DROP TABLE IF EXISTS config_dispatches;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS agent_logs;
DROP TABLE IF EXISTS user_permissions;
DROP TABLE IF EXISTS documents;
DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS devices;
DROP TABLE IF EXISTS agents;
DROP TABLE IF EXISTS factories;
DROP TABLE IF EXISTS users;