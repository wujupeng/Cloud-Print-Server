#!/bin/bash
set -euo pipefail

SERVICE_NAME="cloud-print-server"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
BINARY_PATH="/usr/local/bin/cloud-print-server"
CONFIG_DIR="/etc/cloud-print-server"
DATA_DIR="/var/lib/cloud-print-server"
LOG_DIR="/var/log/cloud-print-server"
RUN_USER="cps"
RUN_GROUP="cps"
DB_PATH="${DATA_DIR}/cloud-print.db"
CONFIG_FILE="${CONFIG_DIR}/config.yaml"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

log_info()  { echo "[INFO]  $*"; }
log_warn()  { echo "[WARN]  $*"; }
log_error() { echo "[ERROR] $*" >&2; }
log_ok()    { echo "[OK]    $*"; }

require_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "此脚本需要 root 权限运行"
        exit 1
    fi
}

user_exists() {
    id "$1" >/dev/null 2>&1
}

ensure_user() {
    if user_exists "${RUN_USER}"; then
        log_info "用户 ${RUN_USER} 已存在"
        return
    fi
    useradd --system --no-create-home --shell /usr/sbin/nologin --home-dir "${DATA_DIR}" "${RUN_USER}"
    log_ok "创建系统用户 ${RUN_USER}"
}

ensure_dirs() {
    install -d -o "${RUN_USER}" -g "${RUN_GROUP}" -m 0750 "${CONFIG_DIR}"
    install -d -o "${RUN_USER}" -g "${RUN_GROUP}" -m 0750 "${DATA_DIR}"
    install -d -o "${RUN_USER}" -g "${RUN_GROUP}" -m 0750 "${DATA_DIR}/data"
    install -d -o "${RUN_USER}" -g "${RUN_GROUP}" -m 0750 "${LOG_DIR}"
    log_ok "目录结构就绪"
}

install_binary() {
    local src="${SCRIPT_DIR}/../bin/${SERVICE_NAME}"
    if [[ ! -f "${src}" ]]; then
        log_warn "未找到编译产物 ${src}，尝试从 ${SCRIPT_DIR}/../cmd/cloud-print-server 构建"
        src="${SCRIPT_DIR}/../cmd/cloud-print-server"
    fi
    if [[ -d "${src}" ]]; then
        log_info "从源码构建二进制"
        (cd "$(dirname "${src}")/.." && go build -o "${BINARY_PATH}" ./cmd/cloud-print-server)
    else
        install -m 0755 "${src}" "${BINARY_PATH}"
    fi
    chown "${RUN_USER}:${RUN_GROUP}" "${BINARY_PATH}"
    log_ok "二进制已安装到 ${BINARY_PATH}"
}

install_config() {
    if [[ ! -f "${CONFIG_FILE}" ]]; then
        if [[ -f "${SCRIPT_DIR}/config.example.yaml" ]]; then
            install -o "${RUN_USER}" -g "${RUN_GROUP}" -m 0640 "${SCRIPT_DIR}/config.example.yaml" "${CONFIG_FILE}"
            log_ok "已部署示例配置 ${CONFIG_FILE}"
        else
            log_warn "缺少示例配置，请手动创建 ${CONFIG_FILE}"
        fi
    else
        log_info "配置文件已存在，跳过"
    fi
}

ensure_secrets() {
    local jwt_secret_file="${CONFIG_DIR}/jwt.secret"
    local master_key_file="${CONFIG_DIR}/master.key"
    if [[ ! -f "${jwt_secret_file}" ]]; then
        openssl rand -hex 32 > "${jwt_secret_file}"
        chmod 0640 "${jwt_secret_file}"
        chown "${RUN_USER}:${RUN_GROUP}" "${jwt_secret_file}"
        log_ok "生成 JWT 密钥"
    fi
    if [[ ! -f "${master_key_file}" ]]; then
        openssl rand -hex 32 > "${master_key_file}"
        chmod 0640 "${master_key_file}"
        chown "${RUN_USER}:${RUN_GROUP}" "${master_key_file}"
        log_ok "生成 Master Key"
    fi
}

init_database() {
    if [[ ! -x "${BINARY_PATH}" ]]; then
        log_warn "二进制不存在，跳过数据库初始化"
        return
    fi
    if [[ -f "${SCRIPT_DIR}/init-db.sh" ]]; then
        log_info "执行数据库初始化"
        CPS_JWT_SECRET="$(cat "${CONFIG_DIR}/jwt.secret")" \
        CPS_MASTER_KEY="$(cat "${CONFIG_DIR}/master.key")" \
        bash "${SCRIPT_DIR}/init-db.sh" "${DB_PATH}" "${SCRIPT_DIR}/../migrations"
    fi
    chown -R "${RUN_USER}:${RUN_GROUP}" "${DATA_DIR}"
    log_ok "数据库初始化完成"
}

install_systemd() {
    if [[ ! -f "${SCRIPT_DIR}/${SERVICE_NAME}.service" ]]; then
        log_error "缺少 ${SERVICE_NAME}.service 单元文件"
        exit 1
    fi
    install -m 0644 "${SCRIPT_DIR}/${SERVICE_NAME}.service" "${SERVICE_FILE}"
    systemctl daemon-reload
    log_ok "systemd 单元已注册"
}

enable_service() {
    systemctl enable "${SERVICE_NAME}" >/dev/null 2>&1
    systemctl restart "${SERVICE_NAME}"
    log_ok "服务已启用并启动"
}

do_install() {
    require_root
    log_info "开始安装 ${SERVICE_NAME}"
    ensure_user
    ensure_dirs
    install_binary
    install_config
    ensure_secrets
    init_database
    install_systemd
    enable_service
    log_ok "安装完成。服务地址: http://127.0.0.1:8080"
}

do_uninstall() {
    require_root
    log_info "开始卸载 ${SERVICE_NAME}"
    systemctl stop "${SERVICE_NAME}" 2>/dev/null || true
    systemctl disable "${SERVICE_NAME}" 2>/dev/null || true
    rm -f "${SERVICE_FILE}"
    systemctl daemon-reload
    rm -f "${BINARY_PATH}"
    log_warn "保留数据目录 ${DATA_DIR} 与配置 ${CONFIG_DIR}，如需清理请手动删除"
    log_ok "卸载完成"
}

do_restart() {
    require_root
    systemctl restart "${SERVICE_NAME}"
    log_ok "服务已重启"
}

do_status() {
    systemctl status "${SERVICE_NAME}" --no-pager || true
    echo "---"
    if systemctl is-active --quiet "${SERVICE_NAME}"; then
        local agent_count task_count
        agent_count=$(curl -s "http://127.0.0.1:8080/api/v1/status" 2>/dev/null | grep -o '"online_agents":[0-9]*' | cut -d: -f2 || echo "N/A")
        task_count=$(curl -s "http://127.0.0.1:8080/api/v1/status" 2>/dev/null | grep -o '"pending_tasks":[0-9]*' | cut -d: -f2 || echo "N/A")
        echo "Agent 连接数: ${agent_count}"
        echo "待处理任务数: ${task_count}"
    else
        log_warn "服务未运行"
    fi
}

do_logs() {
    journalctl -u "${SERVICE_NAME}" -f
}

usage() {
    cat <<EOF
用法: $0 {install|uninstall|restart|status|logs}
  install    安装并启动服务
  uninstall  停止并卸载服务（保留数据）
  restart    重启服务
  status     查看服务状态、Agent 连接数、任务数
  logs       实时查看日志（journalctl -f）
EOF
}

main() {
    local cmd="${1:-}"
    case "${cmd}" in
        install)   do_install ;;
        uninstall) do_uninstall ;;
        restart)   do_restart ;;
        status)    do_status ;;
        logs)      do_logs ;;
        *)         usage; exit 1 ;;
    esac
}

main "$@"