# Cloud Print Server - 云打印平台

[![Version](https://img.shields.io/badge/version-v1.0.0-blue.svg)](https://github.com/wujupeng/Cloud-Print-Server/releases/tag/v1.0.0)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

> 云打印管理平台，提供 Web 管理界面、REST API 和 WebSocket Hub，管理用户、设备、打印任务，并通过 WSS 长连接向工厂端 Agent 分发打印任务。

## 目录

- [系统概述](#系统概述)
- [系统架构](#系统架构)
- [核心功能](#核心功能)
- [项目结构](#项目结构)
- [部署教程](#部署教程)
- [配置说明](#配置说明)
- [API 文档](#api-文档)
- [Web 管理界面](#web-管理界面)
- [使用指南](#使用指南)
- [运维手册](#运维手册)
- [故障排查](#故障排查)
- [版本历史](#版本历史)

---

## 系统概述

Cloud Print Server 是云打印系统的云端核心组件，负责：

- **用户管理**：注册/登录/权限控制（JWT 认证）
- **设备管理**：打印机注册/状态监控/权限分配
- **任务调度**：接收打印任务 → 嵌入文档内容 → 通过 WSS 分发至 Agent
- **文档存储**：文档上传/暂存/校验/自动清理
- **实时通信**：WebSocket Hub 管理 Agent 连接，SSE 推送任务状态
- **Web 界面**：管理控制台（登录/仪表盘/任务/设备/管理员）

### 网络拓扑

```
                         ┌────────────────────────────────────────────────┐
                         │              Cloud Print Server                 │
                         │              (print.oascii.com)                 │
                         │  ┌──────────┐  ┌──────────┐  ┌──────────┐    │
  用户浏览器 ──────────► │  │  Web UI  │  │ REST API │  │  WSS Hub │    │
                         │  └──────────┘  └──────────┘  └────┬─────┘    │
                         │                                   │           │
                         │  ┌──────────────────────────────┼──────┐    │
                         │  │     SQLite + Document Store   │      │    │
                         │  └──────────────────────────────┼──────┘    │
                         └──────────────────────────────────┼──────────┘
                                                            │ WSS/443
                                              ┌─────────────▼──────────┐
                                              │  Cloud Print Agent     │
                                              │  (工厂本地 192.168.2.40)│
                                              └────────────┬──────────┘
                                                           │
                                              ┌────────────▼──────────┐
                                              │     本地打印机群       │
                                              └───────────────────────┘
```

---

## 系统架构

### 组件关系

```
Cloud Print Server (本仓库)
    ├── httpserver     — HTTP 服务器（路由注册/中间件）
    ├── restapi        — REST API 处理器（认证/设备/文档/任务/事件）
    ├── wsshub         — WebSocket Hub（Agent 连接管理/消息收发/任务分发）
    ├── webui          — Web 管理界面（模板渲染/静态资源）
    ├── adminapi       — 管理员 API（用户管理/设备管理/Agent 管理）
    ├── auth           — 认证模块（JWT 签发/验证/中间件）
    ├── taskmanager    — 任务管理（创建/分发/状态更新/取消）
    ├── devicemanager  — 设备管理（CRUD/状态更新/权限）
    ├── agentmanager   — Agent 管理（注册/状态/连接管理）
    ├── docstore       — 文档存储（保存/读取/校验/清理）
    ├── storage        — 数据访问层（SQLite + sqlc）
    ├── domain         — 领域模型（任务/设备/用户/Agent）
    ├── config         — 配置管理
    ├── lifecycle      — 生命周期（systemd/信号处理）
    └── observability  — 可观测性（日志/审计/追踪）

Cloud Print Agent (姊妹仓库: wujupeng/Cloud-Print-Agent)
    部署在工厂本地，接收 Server 分发的任务并控制打印机执行
```

### 技术栈

| 组件 | 技术 |
|------|------|
| 语言 | Go 1.22 |
| HTTP 路由 | chi/v5 |
| WebSocket | nhooyr.io/websocket |
| 数据库 | SQLite (modernc.org/sqlite, 纯 Go 驱动) |
| ORM | sqlc (代码生成) |
| 认证 | JWT (golang-jwt/jwt/v5) |
| 密码 | bcrypt |
| 配置 | Viper + YAML |
| 日志 | zap + lumberjack |
| 迁移 | golang-migrate |
| 前端 | 原生 HTML/CSS/JavaScript |

---

## 核心功能

### 1. 用户认证与权限

- **JWT 认证**：登录获取 Token，12 小时有效期
- **角色控制**：ADMIN（管理员）/ USER（普通用户）
- **设备权限**：用户只能使用已授权的设备
- **Cookie 支持**：Web UI 通过 Cookie 传递 Token，API 通过 Bearer Header

### 2. 设备管理

- **多工厂支持**：一个 Server 可管理多个工厂的设备
- **多协议设备**：RAW/LPR/IPP/CUPS 协议设备统一管理
- **状态监控**：实时更新设备在线/离线状态
- **权限分配**：管理员为用户分配设备访问权限

### 3. 任务调度

- **任务创建**：用户上传文档 → 创建任务 → 自动分发
- **内容嵌入**：Server 读取文档内容嵌入任务消息，Agent 无需回拉
- **状态流转**：PENDING → DISPATCHED → RUNNING → SUCCESS/FAILED
- **自动重试**：Agent 侧失败自动重试，Server 侧记录重试次数
- **任务取消**：支持取消 PENDING/RUNNING 状态的任务

### 4. 实时通信

- **WSS Hub**：管理多个 Agent 的 WebSocket 连接
- **心跳保活**：30s 心跳间隔，90s 超时
- **SSE 事件流**：向 Web UI 实时推送任务状态变更
- **消息信封**：统一 JSON 信封格式，含类型/追踪 ID/时间戳

### 5. 文档管理

- **上传存储**：支持最大 50MB 文档上传
- **校验去重**：SHA-256 校验和
- **自动清理**：按保留时间自动清理过期文档（默认 24 小时）

### 6. Web 管理界面

| 页面 | 路径 | 功能 |
|------|------|------|
| 登录 | `/login` | 用户登录 |
| 仪表盘 | `/dashboard` | 系统概览 |
| 任务管理 | `/tasks` | 任务列表/创建/详情 |
| 新建任务 | `/tasks/new` | 上传文档+选择设备+创建任务 |
| 设备管理 | `/devices` | 设备列表/状态 |
| 管理员 | `/admin` | 用户/设备/Agent 管理（仅管理员） |

---

## 项目结构

```
Cloud-Print-Server/
├── cmd/
│   └── cloud-print-server/       # 主程序入口
├── internal/
│   ├── adminapi/                 # 管理员 API
│   ├── agentmanager/             # Agent 管理
│   ├── auth/                     # 认证模块
│   ├── config/                   # 配置管理
│   ├── configmanager/            # 配置下发管理
│   ├── devicemanager/            # 设备管理
│   ├── docstore/                 # 文档存储
│   ├── domain/                   # 领域模型
│   ├── errs/                     # 错误定义
│   ├── httpserver/               # HTTP 服务器
│   ├── lifecycle/                # 生命周期
│   ├── observability/            # 可观测性
│   ├── restapi/                  # REST API
│   ├── storage/                  # 数据访问层
│   ├── taskmanager/              # 任务管理
│   ├── webui/                    # Web 管理界面
│   └── wsshub/                   # WebSocket Hub
├── web/                          # Web 前端资源
│   ├── static/                   #   静态文件 (CSS/JS)
│   └── templates/                #   HTML 模板
├── deploy/                       # 部署文件
│   ├── cloud-print-server.service #  systemd 服务
│   ├── config.example.yaml       #   配置示例
│   ├── nginx-print.conf          #   Nginx 配置
│   ├── init-db.sh                #   数据库初始化
│   └── cps.sh                    #   运维工具
├── migrations/                   # 数据库迁移
├── test/                         # 测试
├── go.mod                        # Go 模块定义
└── USAGE.md                      # 使用文档
```

---

## 部署教程

### 前置条件

- Linux 服务器（Debian 12+/Ubuntu 22.04+）
- Go 1.22+
- Nginx（反向代理 + TLS）
- 域名（如 print.oascii.com）解析到服务器

### 1. 编译 Server

```bash
git clone https://github.com/wujupeng/Cloud-Print-Server.git
cd Cloud-Print-Server
go build -o cloud-print-server ./cmd/cloud-print-server
```

### 2. 配置 Server

```bash
sudo mkdir -p /etc/cloud-print-server
sudo cp deploy/config.example.yaml /etc/cloud-print-server/config.yaml
sudo nano /etc/cloud-print-server/config.yaml
```

**配置文件：**

```yaml
server:
  listen: ":8080"
  domain: "print.oascii.com"
  tls:
    enabled: false              # TLS 由 Nginx 处理

db:
  path: "/var/lib/cloud-print-server/server.db"

storage:
  data_dir: "/var/lib/cloud-print-server/data"
  doc_retention_hours: 24       # 文档保留时间

auth:
  jwt_ttl_hours: 12             # JWT 有效期
  bcrypt_cost: 10

log:
  level: "info"
  dir: "/var/log/cloud-print-server"

wss:
  path: "/agent"
  heartbeat_interval: 30s
  heartbeat_timeout: 90s

upload:
  max_size_mb: 50               # 最大上传大小
```

### 3. 安装系统服务

```bash
# 创建用户
sudo useradd --system --no-create-home --shell /usr/sbin/nologin cps

# 创建目录
sudo mkdir -p /var/lib/cloud-print-server/data /var/log/cloud-print-server
sudo chown -R cps:cps /var/lib/cloud-print-server /var/log/cloud-print-server

# 安装二进制
sudo cp cloud-print-server /usr/local/bin/

# 安装 systemd 服务
sudo cp deploy/cloud-print-server.service /etc/systemd/system/
sudo systemctl daemon-reload

# 生成密钥
openssl rand -base64 32 | sudo tee /etc/cloud-print-server/jwt.secret
openssl rand -base64 32 | sudo tee /etc/cloud-print-server/master.key
sudo chmod 600 /etc/cloud-print-server/jwt.secret /etc/cloud-print-server/master.key
sudo chown cps:cps /etc/cloud-print-server/jwt.secret /etc/cloud-print-server/master.key

# 启动服务
sudo systemctl enable cloud-print-server
sudo systemctl start cloud-print-server
```

### 4. 配置 Nginx 反向代理

```bash
sudo cp deploy/nginx-print.conf /etc/nginx/sites-available/print.oascii.com
sudo ln -s /etc/nginx/sites-available/print.oascii.com /etc/nginx/sites-enabled/

# 配置 TLS 证书
sudo mkdir -p /etc/nginx/ssl
sudo cp server.crt server.key /etc/nginx/ssl/

# 测试并重载
sudo nginx -t && sudo nginx -s reload
```

**Nginx 配置（`nginx-print.conf`）：**

```nginx
server {
    listen 80;
    server_name print.oascii.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl;
    server_name print.oascii.com;

    ssl_certificate /etc/nginx/ssl/server.crt;
    ssl_certificate_key /etc/nginx/ssl/server.key;
    ssl_protocols TLSv1.2 TLSv1.3;

    client_max_body_size 50m;

    # WebSocket 代理（Agent 连接）
    location /agent {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 86400s;
    }

    # HTTP/API 代理
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

### 5. 初始化数据库

```bash
# 运行数据库迁移
sudo bash deploy/init-db.sh

# 初始化管理员账号
sudo bash deploy/init_data.sh
```

### 6. 部署 Web UI

```bash
sudo cp -r web /etc/cloud-print-server/web
```

### 7. 验证部署

```bash
# 检查服务状态
sudo systemctl status cloud-print-server

# 健康检查
curl http://127.0.0.1:8080/api/v1/healthz

# 登录测试
curl -s -X POST http://127.0.0.1:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'
```

---

## 配置说明

### 完整配置项

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `server.listen` | 监听地址 | :8080 |
| `server.domain` | 域名 | print.oascii.com |
| `server.tls.enabled` | 启用 TLS | false（由 Nginx 处理） |
| `db.path` | SQLite 数据库路径 | /var/lib/cloud-print-server/server.db |
| `storage.data_dir` | 文档存储目录 | /var/lib/cloud-print-server/data |
| `storage.doc_retention_hours` | 文档保留小时数 | 24 |
| `auth.jwt_ttl_hours` | JWT 有效期 | 12 |
| `auth.bcrypt_cost` | bcrypt 计算成本 | 10 |
| `log.level` | 日志级别 | info |
| `log.dir` | 日志目录 | /var/log/cloud-print-server |
| `wss.path` | WebSocket 路径 | /agent |
| `wss.heartbeat_interval` | 心跳间隔 | 30s |
| `wss.heartbeat_timeout` | 心跳超时 | 90s |
| `upload.max_size_mb` | 最大上传大小 | 50 |

---

## API 文档

### 认证

#### POST /api/v1/auth/login

```bash
curl -s -X POST https://print.oascii.com/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'
```

响应：
```json
{
  "code": "OK",
  "data": {
    "token": "eyJhbGci...",
    "expires_at": "2026-08-20T16:43:40Z",
    "user_id": "a2dfc0b5-...",
    "role": "ADMIN",
    "username": "admin"
  }
}
```

#### POST /api/v1/auth/register

注册新用户（仅管理员）。

### 设备管理

#### GET /api/v1/devices

获取当前用户有权限的设备列表。

```bash
curl -s https://print.oascii.com/api/v1/devices \
  -H "Authorization: Bearer <token>"
```

### 文档管理

#### POST /api/v1/documents/upload

```bash
curl -s -X POST https://print.oascii.com/api/v1/documents/upload \
  -H "Authorization: Bearer <token>" \
  -F "file=@document.pdf"
```

响应：
```json
{
  "code": "OK",
  "data": {
    "doc_id": "b5c85e46-...",
    "filename": "document.pdf",
    "size": 69,
    "checksum": "f067cd45-..."
  }
}
```

#### GET /api/v1/documents/{id}/download

下载文档。

### 打印任务

#### POST /api/v1/tasks/

```bash
curl -s -X POST https://print.oascii.com/api/v1/tasks/ \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{
    "device_id": "f993e25b-...",
    "doc_id": "b5c85e46-...",
    "copies": 1,
    "extra": {"paper_size": "A4"}
  }'
```

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| device_id | string | 是 | 目标设备 ID |
| doc_id | string | 是 | 文档 ID |
| copies | int | 否 | 打印份数（默认 1） |
| orientation | string | 否 | portrait/landscape |
| extra | object | 否 | 额外参数（paper_size 等） |

#### GET /api/v1/tasks/

获取任务列表（支持 `limit` 和 `offset` 分页）。

#### GET /api/v1/tasks/{id}/

获取任务详情。

**任务状态流转：**
```
PENDING → DISPATCHED → RUNNING → SUCCESS
                              → FAILED
                              → RETRYING → RUNNING
PENDING/RUNNING → CANCELLED
```

#### DELETE /api/v1/tasks/{id}/

取消任务。

### 事件流

#### GET /api/v1/events

SSE 事件流，实时推送任务状态变更。

### Agent WebSocket

#### WS /agent?agent_id={id}&token={token}

Agent 连接端点，用于任务分发和状态上报。

---

## Web 管理界面

### 页面说明

| 页面 | 路径 | 说明 |
|------|------|------|
| 登录 | `/login` | 用户名密码登录 |
| 仪表盘 | `/dashboard` | 系统概览（设备/任务/Agent 状态） |
| 任务列表 | `/tasks` | 查看所有打印任务 |
| 新建任务 | `/tasks/new` | 上传文档 + 选择设备 + 创建任务 |
| 设备列表 | `/devices` | 查看所有打印机状态 |
| 管理面板 | `/admin` | 用户/设备/Agent 管理（仅管理员） |

### 使用流程

1. **登录**：访问 `https://print.oascii.com/login`，输入用户名密码
2. **新建任务**：点击"新建任务" → 上传文档 → 选择打印机 → 设置参数 → 提交
3. **查看状态**：在任务列表中查看任务状态（PENDING/RUNNING/SUCCESS/FAILED）
4. **管理设备**：管理员可在"管理"页面添加设备、分配权限

---

## 使用指南

### 完整打印流程

```bash
# 1. 登录
TOKEN=$(curl -s -X POST https://print.oascii.com/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}' \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['token'])")

# 2. 上传文档
DOC_ID=$(curl -s -X POST https://print.oascii.com/api/v1/documents/upload \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@document.txt" \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['doc_id'])")

# 3. 创建打印任务
TASK_ID=$(curl -s -X POST https://print.oascii.com/api/v1/tasks/ \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d "{\"device_id\":\"<device_id>\",\"doc_id\":\"$DOC_ID\"}" \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['task_id'])")

# 4. 查询任务状态
curl -s "https://print.oascii.com/api/v1/tasks/$TASK_ID/" \
  -H "Authorization: Bearer $TOKEN"
```

### 标签打印流程

标签打印机（如 Deli888）使用 TSPL 指令，上传 `.tspl` 文件作为文档：

```bash
# 创建 TSPL 指令文件
cat > label.tspl << 'EOF'
SIZE 80 mm,50 mm
GAP 2 mm,0 mm
CLS
TEXT 10,10,"3",0,1,1,"标签内容"
BARCODE 10,50,"128",80,2,0,2,2,"1234567890"
PRINT 1,1
EOF

# 上传并打印
curl -s -X POST https://print.oascii.com/api/v1/documents/upload \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@label.tspl"
```

---

## 运维手册

### 服务管理

```bash
# 启动/停止/重启
sudo systemctl start cloud-print-server
sudo systemctl stop cloud-print-server
sudo systemctl restart cloud-print-server

# 状态查看
sudo systemctl status cloud-print-server

# 日志查看
sudo journalctl -u cloud-print-server -f                    # 实时日志
sudo journalctl -u cloud-print-server --since "1 hour ago"  # 最近1小时
sudo journalctl -u cloud-print-server -p err                # 仅错误
```

### 数据库管理

```bash
# 查看数据库
sudo sqlite3 /var/lib/cloud-print-server/server.db

# 查看所有表
.tables

# 查看设备
SELECT * FROM devices;

# 查看任务
SELECT task_id, status, device_id, created_at FROM tasks ORDER BY created_at DESC LIMIT 10;

# 查看用户
SELECT user_id, username, role FROM users;

# 添加设备权限
INSERT INTO user_devices (user_id, device_id) VALUES ('<user_id>', '<device_id>');
```

### 添加新设备

```sql
-- 1. 添加设备记录
INSERT INTO devices (device_id, name, ip, model, protocol, status, factory_id, agent_id, port)
VALUES ('<uuid>', '打印机名称', '192.168.x.x', '型号', 'RAW', 'OFFLINE',
        '<factory_id>', '<agent_id>', 9100);

-- 2. 分配用户权限
INSERT INTO user_devices (user_id, device_id)
VALUES ('<user_id>', '<device_id>');
```

### 添加新用户

```bash
# 通过 API 注册
curl -s -X POST https://print.oascii.com/api/v1/auth/register \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{"username":"newuser","password":"password123","role":"USER"}'
```

### 文档清理

文档按 `doc_retention_hours`（默认 24 小时）自动清理。手动清理：

```bash
# 查看文档目录大小
du -sh /var/lib/cloud-print-server/data/

# 手动清理过期文档
find /var/lib/cloud-print-server/data/ -type f -mtime +1 -delete
```

### 备份

```bash
# 备份数据库和文档
sudo tar -czf cloud-print-server-backup-$(date +%Y%m%d).tar.gz \
  /var/lib/cloud-print-server/ \
  /etc/cloud-print-server/
```

---

## 故障排查

### 1. 服务无法启动

```bash
# 查看启动日志
sudo journalctl -u cloud-print-server -n 50

# 常见原因：
# - 数据库文件权限：chown cps:cps /var/lib/cloud-print-server/server.db
# - 配置文件错误：检查 YAML 语法
# - 端口被占用：lsof -i :8080
# - 密钥文件缺失：检查 /etc/cloud-print-server/jwt.secret
```

### 2. Agent 无法连接

```bash
# 检查 WSS 端点
curl -s https://print.oascii.com/api/v1/healthz

# 检查 Nginx WebSocket 代理
sudo nginx -t
sudo tail -f /var/log/nginx/error.log

# 常见原因：
# - Nginx 未配置 WebSocket 升级
# - 防火墙阻断 443 端口
# - Agent Token 无效
```

### 3. 任务状态不更新

```bash
# 检查 Agent 连接状态
sudo sqlite3 /var/lib/cloud-print-server/server.db \
  "SELECT agent_id, status, last_seen_at FROM agents;"

# 检查任务状态
sudo sqlite3 /var/lib/cloud-print-server/server.db \
  "SELECT task_id, status, error_code, error_msg FROM tasks WHERE task_id='<task_id>';"

# 查看 Server 日志
sudo journalctl -u cloud-print-server | grep "<task_id>"
```

### 4. Web UI 401 错误

**原因**：JWT 中间件未从 Cookie 提取 Token。

**解决**：确认 Server 版本支持 Cookie 认证（`extractBearer` 支持 Cookie 回退）。

### 5. 文档上传失败

```bash
# 检查上传大小限制
grep max_size /etc/cloud-print-server/config.yaml

# 检查磁盘空间
df -h /var/lib/cloud-print-server/

# 检查 Nginx 限制
grep client_max_body_size /etc/nginx/nginx.conf
```

---

## 版本历史

### v1.0.0 (2026-08-20)

**核心功能：**
- Web 管理界面（登录/仪表盘/任务/设备/管理员）
- REST API（认证/设备/文档/任务/事件）
- WebSocket Hub（Agent 连接管理/任务分发）
- JWT 认证（Bearer Header + Cookie 支持）
- 文档上传与存储（SHA-256 校验/自动清理）
- SQLite 数据库（sqlc 代码生成）
- 任务调度（文档内容嵌入分发）
- SSE 事件流（实时状态推送）
- systemd 沙箱（严格安全限制）
- Nginx 反向代理配置

**验证：**
- 三台打印机端到端测试全部通过
  - EPSON LQ-630KII (RAW 协议)
  - Canon iR-ADV C3530 (CUPS + UFR II)
  - Deli888 标签打印机 (RAW + TSPL)

---

## 相关仓库

- [Cloud-Print-Agent](https://github.com/wujupeng/Cloud-Print-Agent) — 工厂端云打印代理（WSS 客户端 + 多协议打印）

## 许可证

MIT License