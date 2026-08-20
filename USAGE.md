# 云打印系统 使用文档

## 1. 系统概述

云打印系统（Cloud Print System）是一个分布式打印管理平台，支持用户通过 Web API 远程控制工厂本地的打印机执行打印任务。

### 1.1 架构

```
用户/Web UI → 云打印平台 (print.oascii.com) → 网关 (210.22.123.254) → Agent (192.168.2.40) → 打印设备 (192.168.2.81:9100)
```

### 1.2 组件

| 组件 | 说明 | 部署位置 |
|------|------|----------|
| Cloud Print Server | 云端服务，提供 REST API / WebSocket / Web UI | 192.168.2.40:8080 |
| Cloud Print Agent | 工厂本地代理，接收任务并控制打印机 | 192.168.2.40 |
| Nginx | 反向代理，TLS 终止 | 192.168.2.40:80/443 |

### 1.3 技术栈

- **后端**: Go 1.22+, chi v5 (路由), nhooyr.io/websocket (WSS), SQLite (数据库)
- **前端**: HTML/CSS/JavaScript (原生)
- **部署**: systemd 托管, Nginx 反向代理

---

## 2. 快速开始

### 2.1 管理员登录

```bash
curl -s https://print.oascii.com/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin123"}'
```

响应：
```json
{
  "code": "OK",
  "data": {
    "token": "eyJhbGci...",
    "expires_at": "2026-08-20T11:46:17Z",
    "user_id": "a2dfc0b5-...",
    "role": "ADMIN",
    "username": "admin"
  }
}
```

### 2.2 上传文档

```bash
curl -s https://print.oascii.com/api/v1/documents/upload \
  -H "Authorization: Bearer <token>" \
  -F 'file=@document.pdf;type=application/pdf'
```

### 2.3 创建打印任务

```bash
curl -s https://print.oascii.com/api/v1/tasks/ \
  -H "Authorization: Bearer <token>" \
  -H 'Content-Type: application/json' \
  -d '{
    "device_id": "3aec85cd-8819-4cc0-8dee-343a166cca33",
    "doc_id": "<doc_id>",
    "params": {
      "copies": 1,
      "paper_size": "A4"
    }
  }'
```

### 2.4 查询任务状态

```bash
curl -s https://print.oascii.com/api/v1/tasks/<task_id>/ \
  -H "Authorization: Bearer <token>"
```

---

## 3. API 文档

### 3.1 认证

#### POST /api/v1/auth/login

登录获取 JWT Token。

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| username | string | 是 | 用户名 |
| password | string | 是 | 密码 |

**响应**: `{ "code": "OK", "data": { "token": "...", "expires_at": "...", "user_id": "...", "role": "...", "username": "..." } }`

#### POST /api/v1/auth/register

注册新用户。

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| username | string | 是 | 用户名 |
| password | string | 是 | 密码 |
| role | string | 否 | 角色（USER/ADMIN，默认 USER）|

### 3.2 设备管理

#### GET /api/v1/devices

获取当前用户有权限的设备列表。

**请求头**: `Authorization: Bearer <token>`

**响应**: `{ "code": "OK", "data": [{ "device_id": "...", "name": "...", "ip": "...", "port": 9100, "protocol": "RAW", "status": "ONLINE" }] }`

### 3.3 文档管理

#### POST /api/v1/documents/upload

上传文档。

**请求头**: `Authorization: Bearer <token>`

**请求体**: `multipart/form-data`, 字段名 `file`

**响应**: `{ "code": "OK", "data": { "doc_id": "...", "filename": "...", "size": 18, "checksum": "..." } }`

#### GET /api/v1/documents/{id}/download

下载文档。

**路径参数**: `id` - 文档 ID

### 3.4 打印任务

#### POST /api/v1/tasks/

创建打印任务（自动分发到 Agent）。

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| device_id | string | 是 | 目标设备 ID |
| doc_id | string | 是 | 文档 ID |
| params.copies | int | 否 | 打印份数（默认 1）|
| params.paper_size | string | 否 | 纸张大小（A4/A5/Letter）|
| params.orientation | string | 否 | 方向（portrait/landscape）|

**响应**: `{ "code": "OK", "data": { "task_id": "...", "status": "PENDING", ... } }`

#### GET /api/v1/tasks/

获取任务列表。

| 查询参数 | 类型 | 说明 |
|----------|------|------|
| limit | int | 每页数量（默认 50）|
| offset | int | 偏移量 |

#### GET /api/v1/tasks/{id}/

获取任务详情。

**任务状态流转**: `PENDING → DISPATCHED → RUNNING → SUCCESS/FAILED`

#### DELETE /api/v1/tasks/{id}/

取消任务（仅 PENDING/RUNNING 状态可取消）。

### 3.5 事件流

#### GET /api/v1/events

SSE 事件流，实时推送任务状态变更。

**事件类型**: `task_status`, `device_status`, `agent_status`

### 3.6 健康检查

#### GET /api/v1/healthz

**响应**: `{ "status": "ok" }`

#### GET /api/v1/status

系统状态信息。

### 3.7 Agent WebSocket

#### WS /agent?agent_id={id}&token={token}

Agent 连接端点，用于任务分发和状态上报。

**消息类型**:
- `auth_ok` - 认证成功握手
- `task` - 任务下发
- `task_ack` - Agent 任务确认
- `task_result` - Agent 任务结果上报
- `heartbeat` - 心跳
- `heartbeat_ack` - 心跳回复
- `control` - 控制指令（取消任务等）

---

## 4. 用户使用流程

### 4.1 普通用户

1. **登录**: `POST /api/v1/auth/login` 获取 JWT Token
2. **查看设备**: `GET /api/v1/devices` 获取可用打印机列表
3. **上传文档**: `POST /api/v1/documents/upload` 上传要打印的文件
4. **创建任务**: `POST /api/v1/tasks/` 创建打印任务（系统自动分发到 Agent）
5. **查询状态**: `GET /api/v1/tasks/{id}/` 轮询任务状态，或通过 `GET /api/v1/events` SSE 实时监听
6. **取消任务**（可选）: `DELETE /api/v1/tasks/{id}/` 取消未完成的任务

### 4.2 管理员

除普通用户操作外，管理员还可以：

1. **注册用户**: `POST /api/v1/auth/register` 创建新用户
2. **管理设备**: 添加/删除/更新设备信息
3. **管理 Agent**: 注册新 Agent，查看 Agent 状态
4. **查看所有任务**: 管理员可查看所有用户的任务

---

## 5. Agent 配置说明

### 5.1 配置文件

配置文件路径: `/etc/cloud-print-agent/config.yaml`

```yaml
cloud:
  endpoint: print.oascii.com
  port: 8080
  protocol: ws
  agent_id: 7dc67d95-5787-4b57-8d7d-c66f6ea68956

devices:
  - device_id: 3aec85cd-8819-4cc0-8dee-343a166cca33
    name: EPSON-LQ630KII
    ip: 192.168.2.81
    port: 9100
    protocol: RAW

heartbeat:
  interval: 30s
  timeout: 90s

retry:
  max_count: 3
  init_backoff: 5s
  max_backoff: 60s

queue:
  capacity: 100
```

### 5.2 环境变量

环境变量文件: `/etc/cloud-print-agent/credentials.env`

| 变量 | 说明 |
|------|------|
| CPA_MASTER_KEY | 凭证加密主密钥（base64）|
| CPA_CLOUD_PORT | Server 端口（覆盖配置文件）|
| CPA_CLOUD_PROTOCOL | 连接协议 ws/wss |
| CPA_CLOUD_AGENT_ID | Agent ID |
| CPA_CLOUD_SKIP_TLS_VERIFY | 是否跳过 TLS 验证 |

### 5.3 日志

日志路径: `/var/log/cloud-print-agent/agent.log`

```bash
sudo journalctl -u cloud-print-agent -f
```

---

## 6. 部署指南

### 6.1 Server 部署

```bash
# 二进制位置
/usr/local/bin/cloud-print-server

# 配置文件
/etc/cloud-print-server/config.yaml

# 数据库
/var/lib/cloud-print-server/server.db

# systemd 服务
sudo systemctl start/stop/restart cloud-print-server
sudo systemctl status cloud-print-server

# 查看日志
sudo journalctl -u cloud-print-server -f
```

### 6.2 Agent 部署

```bash
# 二进制位置
/usr/local/bin/cloud-print-agent

# 配置文件
/etc/cloud-print-agent/config.yaml

# systemd 服务
sudo systemctl start/stop/restart cloud-print-agent
sudo systemctl status cloud-print-agent

# 查看日志
sudo journalctl -u cloud-print-agent -f
```

### 6.3 Nginx 配置

```bash
# 配置文件
/etc/nginx/sites-available/print.oascii.com

# TLS 证书
/etc/nginx/ssl/server.crt
/etc/nginx/ssl/server.key

# 重载配置
sudo nginx -t && sudo nginx -s reload
```

---

## 7. 系统数据

### 7.1 默认账号

| 用户名 | 密码 | 角色 |
|--------|------|------|
| admin | admin123 | ADMIN |

### 7.2 已注册设备

| 设备名称 | 设备 ID | IP | 端口 | 协议 |
|----------|---------|-----|------|------|
| EPSON LQ-630KII | 3aec85cd-8819-4cc0-8dee-343a166cca33 | 192.168.2.81 | 9100 | RAW |

### 7.3 已注册 Agent

| Agent 名称 | Agent ID | 所属工厂 |
|------------|----------|----------|
| 宝山Agent | 7dc67d95-5787-4b57-8d7d-c66f6ea68956 | 宝山工厂 |

---

## 8. 故障排查

### 8.1 Agent 无法连接 Server

```bash
# 检查 Agent 状态
sudo systemctl status cloud-print-agent

# 检查 Agent 日志
sudo journalctl -u cloud-print-agent --since '10 min ago'

# 检查网络连通性
curl -s http://127.0.0.1:8080/api/v1/healthz
```

### 8.2 打印任务失败

```bash
# 查看任务状态
sudo sqlite3 /var/lib/cloud-print-server/server.db \
  "SELECT task_id, status, error_code, error_msg FROM tasks WHERE task_id='<task_id>';"

# 查看 Agent 执行日志
sudo grep '<task_id>' /var/log/cloud-print-agent/agent.log
```

### 8.3 打印机不可达

```bash
# 测试打印机连通性
nc -zv 192.168.2.81 9100

# 检查设备状态
curl -s http://127.0.0.1:8080/api/v1/devices \
  -H "Authorization: Bearer <token>"
```