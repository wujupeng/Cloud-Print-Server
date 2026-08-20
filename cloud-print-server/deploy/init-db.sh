#!/bin/bash
set -euo pipefail

DB_PATH="${1:-/var/lib/cloud-print-server/cloud-print.db}"
MIGRATIONS_DIR="${2:-./migrations}"

ADMIN_USERNAME="${ADMIN_USERNAME:-admin}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-ChangeMe@2024}"
ADMIN_DISPLAY_NAME="${ADMIN_DISPLAY_NAME:-系统管理员}"

DEFAULT_FACTORY_ID="${DEFAULT_FACTORY_ID:-factory-default}"
DEFAULT_FACTORY_NAME="${DEFAULT_FACTORY_NAME:-默认工厂}"
DEFAULT_FACTORY_CODE="${DEFAULT_FACTORY_CODE:-DEFAULT}"
DEFAULT_FACTORY_LOCATION="${DEFAULT_FACTORY_LOCATION:-上海}"

BAOSHAN_AGENT_ID="${BAOSHAN_AGENT_ID:-agent-baoshan}"
BAOSHAN_AGENT_NAME="${BAOSHAN_AGENT_NAME:-宝山 Agent}"
BAOSHAN_AGENT_VERSION="${BAOSHAN_AGENT_VERSION:-0.1.0}"

log_info()  { echo "[INFO]  $*"; }
log_ok()    { echo "[OK]    $*"; }
log_warn()  { echo "[WARN]  $*"; }
log_error() { echo "[ERROR] $*" >&2; }

require_sqlite() {
    if ! command -v sqlite3 >/dev/null 2>&1; then
        log_error "未找到 sqlite3 命令"
        exit 1
    fi
}

require_bcrypt() {
    if ! command -v python3 >/dev/null 2>&1; then
        log_error "未找到 python3，无法生成 bcrypt 哈希"
        exit 1
    fi
    if ! python3 -c "import bcrypt" 2>/dev/null; then
        log_warn "python3 bcrypt 模块未安装，尝试安装"
        pip3 install bcrypt >/dev/null 2>&1 || {
            log_warn "pip 安装 bcrypt 失败，将使用明文占位（请上线前替换）"
        }
    fi
}

hash_password() {
    local password="$1"
    python3 -c "
import bcrypt
print(bcrypt.hashpw('${password}'.encode(), bcrypt.gensalt(10)).decode())
" 2>/dev/null || echo "PLAIN:${password}"
}

run_migrations() {
    if [[ ! -d "${MIGRATIONS_DIR}" ]]; then
        log_warn "迁移目录 ${MIGRATIONS_DIR} 不存在，跳过"
        return
    fi
    for f in "${MIGRATIONS_DIR}"/*.up.sql; do
        [[ -f "$f" ]] || continue
        local version
        version="$(basename "$f" .up.sql)"
        log_info "应用迁移 ${version}"
        sqlite3 "${DB_PATH}" < "$f"
    done
    log_ok "数据库迁移完成"
}

ensure_schema_versions() {
    sqlite3 "${DB_PATH}" "CREATE TABLE IF NOT EXISTS schema_versions (version TEXT PRIMARY KEY, applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP);"
    for f in "${MIGRATIONS_DIR}"/*.up.sql; do
        [[ -f "$f" ]] || continue
        local version
        version="$(basename "$f" .up.sql)"
        sqlite3 "${DB_PATH}" "INSERT OR IGNORE INTO schema_versions (version) VALUES ('${version}');"
    done
}

create_admin_user() {
    local existing
    existing=$(sqlite3 "${DB_PATH}" "SELECT COUNT(*) FROM users WHERE username='${ADMIN_USERNAME}';")
    if [[ "${existing}" -gt 0 ]]; then
        log_info "管理员用户 ${ADMIN_USERNAME} 已存在，跳过"
        return
    fi
    local admin_id="user-admin"
    local pw_hash
    pw_hash=$(hash_password "${ADMIN_PASSWORD}")
    sqlite3 "${DB_PATH}" "INSERT INTO users (user_id, username, password_hash, password_salt, role, status, display_name, created_at, updated_at) VALUES ('${admin_id}', '${ADMIN_USERNAME}', '${pw_hash}', '', 'ADMIN', 'ACTIVE', '${ADMIN_DISPLAY_NAME}', datetime('now'), datetime('now'));"
    log_ok "管理员用户已创建: ${ADMIN_USERNAME} (角色 ADMIN)"
}

create_default_factory() {
    local existing
    existing=$(sqlite3 "${DB_PATH}" "SELECT COUNT(*) FROM factories WHERE factory_id='${DEFAULT_FACTORY_ID}';")
    if [[ "${existing}" -gt 0 ]]; then
        log_info "默认工厂已存在，跳过"
        return
    fi
    sqlite3 "${DB_PATH}" "INSERT INTO factories (factory_id, name, code, location, created_at, updated_at) VALUES ('${DEFAULT_FACTORY_ID}', '${DEFAULT_FACTORY_NAME}', '${DEFAULT_FACTORY_CODE}', '${DEFAULT_FACTORY_LOCATION}', datetime('now'), datetime('now'));"
    log_ok "默认工厂已创建: ${DEFAULT_FACTORY_NAME}"
}

register_baoshan_agent() {
    local existing
    existing=$(sqlite3 "${DB_PATH}" "SELECT COUNT(*) FROM agents WHERE agent_id='${BAOSHAN_AGENT_ID}';")
    if [[ "${existing}" -gt 0 ]]; then
        log_info "宝山 Agent 已注册，跳过"
        return
    fi
    local token_enc
    if [[ -n "${CPS_MASTER_KEY:-}" ]]; then
        token_enc=$(python3 -c "
import hmac, hashlib, os, binascii
master='${CPS_MASTER_KEY}'.encode()
agent='${BAOSHAN_AGENT_ID}'.encode()
raw=os.urandom(32)
token=binascii.hexlify(raw).decode()
h=hmac.new(master, agent+b':'+token.encode(), hashlib.sha256).hexdigest()
print(token + '|' + h)
" 2>/dev/null || echo "")
    fi
    local token_value token_hash
    if [[ -z "${token_enc}" ]]; then
        token_value="placeholder-token-baoshan"
        token_hash="placeholder-enc-baoshan"
    else
        token_value="${token_enc%%|*}"
        token_hash="${token_enc##*|}"
    fi
    sqlite3 "${DB_PATH}" "INSERT INTO agents (agent_id, name, factory_id, device_token_enc, version, online, online_devices, pending_tasks, net_class, last_heartbeat_at, created_at, updated_at) VALUES ('${BAOSHAN_AGENT_ID}', '${BAOSHAN_AGENT_NAME}', '${DEFAULT_FACTORY_ID}', X'$(printf '%s' "${token_hash}" | head -c 64)', '${BAOSHAN_AGENT_VERSION}', 0, 0, 0, 'OK', NULL, datetime('now'), datetime('now'));"
    log_ok "宝山 Agent 已注册: ${BAOSHAN_AGENT_ID}"
    log_info "Agent Token (请妥善保存): ${token_value}"
}

main() {
    log_info "数据库初始化开始: ${DB_PATH}"
    require_sqlite
    require_bcrypt
    run_migrations
    ensure_schema_versions
    create_admin_user
    create_default_factory
    register_baoshan_agent
    log_ok "数据库初始化完成"
    log_info "管理员账号: ${ADMIN_USERNAME} / ${ADMIN_PASSWORD}"
}

main "$@"