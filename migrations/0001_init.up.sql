-- 0001_init.up.sql: 初始 schema（11 张表）

CREATE TABLE IF NOT EXISTS users (
    user_id        TEXT PRIMARY KEY,
    username       TEXT NOT NULL UNIQUE,
    password_hash  TEXT NOT NULL,
    password_salt  TEXT NOT NULL,
    role           TEXT NOT NULL DEFAULT 'USER',
    status         TEXT NOT NULL DEFAULT 'ACTIVE',
    display_name   TEXT,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS factories (
    factory_id  TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    code        TEXT,
    location    TEXT,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS agents (
    agent_id          TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    factory_id        TEXT NOT NULL,
    device_token_enc  BLOB NOT NULL,
    version           TEXT,
    online            INTEGER NOT NULL DEFAULT 0,
    online_devices    INTEGER NOT NULL DEFAULT 0,
    pending_tasks     INTEGER NOT NULL DEFAULT 0,
    net_class         TEXT,
    last_heartbeat_at DATETIME,
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (factory_id) REFERENCES factories(factory_id)
);

CREATE TABLE IF NOT EXISTS devices (
    device_id      TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    ip             TEXT NOT NULL,
    hostname       TEXT,
    model          TEXT NOT NULL,
    protocol       TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'OFFLINE',
    factory_id     TEXT NOT NULL,
    agent_id       TEXT NOT NULL,
    port           INTEGER,
    last_probe_at  DATETIME,
    last_status_at DATETIME,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (factory_id) REFERENCES factories(factory_id),
    FOREIGN KEY (agent_id) REFERENCES agents(agent_id)
);

CREATE TABLE IF NOT EXISTS tasks (
    task_id        TEXT PRIMARY KEY,
    user_id        TEXT NOT NULL,
    device_id      TEXT NOT NULL,
    agent_id       TEXT,
    doc_id         TEXT,
    document_ref   TEXT,
    checksum       TEXT,
    params         TEXT,
    status         TEXT NOT NULL DEFAULT 'PENDING',
    retry_count    INTEGER NOT NULL DEFAULT 0,
    trace_id       TEXT,
    submitted_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    dispatched_at  DATETIME,
    received_at    DATETIME,
    started_at     DATETIME,
    finished_at    DATETIME,
    next_retry_at  DATETIME,
    error_code     TEXT,
    error_msg      TEXT,
    FOREIGN KEY (user_id) REFERENCES users(user_id),
    FOREIGN KEY (device_id) REFERENCES devices(device_id),
    FOREIGN KEY (agent_id) REFERENCES agents(agent_id)
);

CREATE TABLE IF NOT EXISTS documents (
    doc_id        TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL,
    filename      TEXT NOT NULL,
    content_type  TEXT,
    size          INTEGER NOT NULL,
    checksum      TEXT NOT NULL,
    storage_path  TEXT NOT NULL,
    cleanup_at    DATETIME,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(user_id)
);

CREATE TABLE IF NOT EXISTS user_permissions (
    permission_id TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL,
    device_id     TEXT NOT NULL,
    granted_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, device_id),
    FOREIGN KEY (user_id) REFERENCES users(user_id),
    FOREIGN KEY (device_id) REFERENCES devices(device_id)
);

CREATE TABLE IF NOT EXISTS agent_logs (
    log_id    TEXT PRIMARY KEY,
    agent_id  TEXT NOT NULL,
    level     TEXT NOT NULL,
    event     TEXT NOT NULL,
    message   TEXT,
    trace_id  TEXT,
    ts        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (agent_id) REFERENCES agents(agent_id)
);

CREATE TABLE IF NOT EXISTS audit_logs (
    audit_id TEXT PRIMARY KEY,
    user_id  TEXT,
    action   TEXT NOT NULL,
    target   TEXT,
    detail   TEXT,
    ip       TEXT,
    ts       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS config_dispatches (
    dispatch_id    TEXT PRIMARY KEY,
    agent_id       TEXT NOT NULL,
    config_version INTEGER NOT NULL,
    patch          TEXT,
    applied        INTEGER NOT NULL DEFAULT 0,
    reason         TEXT,
    dispatched_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    acked_at       DATETIME,
    FOREIGN KEY (agent_id) REFERENCES agents(agent_id)
);

CREATE TABLE IF NOT EXISTS config_meta (
    key   TEXT PRIMARY KEY,
    value TEXT
);

CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_agents_factory_id ON agents(factory_id);
CREATE INDEX IF NOT EXISTS idx_devices_agent_status ON devices(agent_id, status);
CREATE INDEX IF NOT EXISTS idx_tasks_user_submitted ON tasks(user_id, submitted_at);
CREATE INDEX IF NOT EXISTS idx_tasks_status_retry ON tasks(status, next_retry_at);
CREATE INDEX IF NOT EXISTS idx_tasks_agent_status ON tasks(agent_id, status);
CREATE INDEX IF NOT EXISTS idx_documents_cleanup ON documents(cleanup_at);
CREATE INDEX IF NOT EXISTS idx_agent_logs_agent_ts ON agent_logs(agent_id, ts);
CREATE INDEX IF NOT EXISTS idx_agent_logs_ts ON agent_logs(ts);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_ts ON audit_logs(user_id, ts);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action_ts ON audit_logs(action, ts);
CREATE INDEX IF NOT EXISTS idx_config_dispatches_agent_ver ON config_dispatches(agent_id, config_version);