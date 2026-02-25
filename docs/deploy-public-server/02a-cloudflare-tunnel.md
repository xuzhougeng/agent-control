# 公网服务器部署（Part 2a）：方案 C Cloudflare Tunnel

> 适用：服务器无公网 IP、不想维护 Nginx/TLS 证书、希望零端口暴露。
> 前提：需要一个托管在 Cloudflare 的域名（Free 计划即可）。

## 方案概述

Cloudflare Tunnel（`cloudflared`）在服务器与 Cloudflare 边缘之间建立出站加密隧道，无需开放任何入站端口。

```
浏览器/Agent ──HTTPS/WSS──▶ Cloudflare 边缘 ──Tunnel──▶ cc-control (127.0.0.1:18080)
```

- 无需公网 IP、无需 Nginx、无需手动管理 TLS 证书
- WebSocket 原生支持，`cc-agent` 可直接通过 `wss://` 连接
- Cloudflare 自带 DDoS 防护

## C.1 前置条件

- 域名已托管到 Cloudflare（NS 已切换，状态为 Active）
- 服务器可访问外网（出站 HTTPS）
- 具备 `sudo` 权限
- 已准备 `cc-control`、`cc-agent` 源码

## C.2 编译与部署 cc-control

编译步骤同方案 B，关键点：cc-control 仅监听 `127.0.0.1`。

```bash
# 编译
cd cc-control
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cc-control ./cmd/cc-control
cd ../cc-agent
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cc-agent ./cmd/cc-agent

# 上传 control 到服务器
scp cc-control root@your-server:/opt/cc-control/
scp -r ../cc-web root@your-server:/opt/cc-control/cc-web
```

## C.3 生成 Admin Token 并启动 cc-control

```bash
ADMIN_TOKEN=$(openssl rand -hex 32)
echo "ADMIN_TOKEN=$ADMIN_TOKEN"
```

`/etc/systemd/system/cc-control.service`：

```ini
[Unit]
Description=CC Control Plane
After=network.target

[Service]
Type=simple
User=cc
Group=cc
WorkingDirectory=/opt/cc-control
ExecStart=/opt/cc-control/cc-control \
  -addr 127.0.0.1:18080 \
  -ui-dir /opt/cc-control/cc-web \
  -admin-token ${ADMIN_TOKEN} \
  -audit-path /opt/cc-control/audit.jsonl \
  -offline-after-sec 30
EnvironmentFile=/opt/cc-control/.env
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

`/opt/cc-control/.env`（600）：

```bash
ADMIN_TOKEN=<your-admin-token>
# optional: persist tokens across restarts
TOKEN_DB=/opt/cc-control/tokens.db
```

```bash
useradd -r -s /sbin/nologin cc
chown -R cc:cc /opt/cc-control
chmod 600 /opt/cc-control/.env
systemctl daemon-reload
systemctl enable --now cc-control
```

可选：增加 `-enable-prompt-detection`。

## C.4 安装 cloudflared

```bash
# Debian/Ubuntu
curl -L https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64.deb -o cloudflared.deb
dpkg -i cloudflared.deb

# 或 RHEL/CentOS
curl -L https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-x86_64.rpm -o cloudflared.rpm
rpm -i cloudflared.rpm
```

## C.5 创建 Tunnel

```bash
# 登录 Cloudflare（会打开浏览器授权，无桌面环境则复制 URL）
cloudflared tunnel login

# 创建 tunnel
cloudflared tunnel create cc-control

# 记录输出中的 Tunnel ID，例如：
# Created tunnel cc-control with id a1b2c3d4-xxxx-xxxx-xxxx-xxxxxxxxxxxx
```

## C.6 配置 Tunnel

创建配置文件 `/opt/cc-control/cloudflared.yml`：

```yaml
tunnel: a1b2c3d4-xxxx-xxxx-xxxx-xxxxxxxxxxxx   # 替换为你的 Tunnel ID
credentials-file: /root/.cloudflared/a1b2c3d4-xxxx-xxxx-xxxx-xxxxxxxxxxxx.json

ingress:
  - hostname: cc.example.com        # 替换为你的域名
    service: http://127.0.0.1:18080
    originRequest:
      noTLSVerify: false
  - service: http_status:404
```

添加 DNS 记录（自动创建 CNAME 指向 Tunnel）：

```bash
cloudflared tunnel route dns cc-control cc.example.com
```

## C.7 启动 Tunnel（Systemd）

```bash
cloudflared service install --config /opt/cc-control/cloudflared.yml
systemctl enable --now cloudflared
```

验证 Tunnel 状态：

```bash
cloudflared tunnel info cc-control
systemctl status cloudflared
```

## C.8 创建 Tenant Token（Admin API）

```bash
curl -X POST https://cc.example.com/admin/tokens \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"type":"tenant"}'
```

## C.9 创建 UI/Agent Token（Tenant API）

```bash
curl -X POST https://cc.example.com/tenant/tokens \
  -H "Authorization: Bearer <tenant-token>" \
  -H "Content-Type: application/json" \
  -d '{"role":"owner"}'
```

说明：每次调用会撤销该租户旧的 UI/Agent token，请同步更新浏览器和 agent 的配置。

## C.10 部署 cc-agent

cc-agent 通过 Cloudflare Tunnel 连接，使用受信任的 TLS 证书，无需 `-tls-skip-verify`。

```bash
/opt/cc-agent/cc-agent \
  -control-url wss://cc.example.com/ws/agent \
  -agent-token "<agent-token>" \
  -server-id srv-gpu-01 \
  -allow-root /home/deploy/repos \
  -claude-path /path/to/ai-cli
```

Systemd `/etc/systemd/system/cc-agent.service`：

```ini
[Unit]
Description=CC Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=deploy
Group=deploy
WorkingDirectory=/opt/cc-agent
ExecStart=/opt/cc-agent/cc-agent \
  -control-url wss://cc.example.com/ws/agent \
  -agent-token ${AGENT_TOKEN} \
  -server-id ${SERVER_ID} \
  -allow-root /home/deploy/repos \
  -claude-path /path/to/ai-cli
EnvironmentFile=/opt/cc-agent/.env
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

`/opt/cc-agent/.env`（600）：

```bash
AGENT_TOKEN=<agent-token-from-tenant-api>
SERVER_ID=srv-gpu-01
```

## C.11 验证

```bash
# Tunnel 状态
cloudflared tunnel info cc-control

# agent 连接日志
journalctl -u cc-agent -f

# 浏览器访问
# https://cc.example.com
```

使用 UI token 登录。

## C.12 可选：Cloudflare Access 零信任门控

可在 Cloudflare Zero Trust 控制台为 `cc.example.com` 添加 Access 策略，实现额外身份验证层：

1. 进入 Cloudflare Dashboard → Zero Trust → Access → Applications
2. 添加 Self-hosted Application，域名填 `cc.example.com`
3. 配置策略（如邮箱 OTP、GitHub OAuth 等）

启用后，用户在浏览器访问时需先通过 Cloudflare Access 认证，再使用 UI token 登录。

注意：Access 策略会拦截所有请求（包括 `/ws/agent`）。若 agent 在 Access 策略保护范围内，需为 agent 配置 Service Token 绕过：

1. Zero Trust → Access → Service Auth → Create Service Token
2. 在 Access 策略中添加 Bypass 规则：Service Token 匹配
3. Agent 需通过 HTTP header 传递 Service Token（需 cc-agent 支持自定义 header，否则建议将 `/ws/agent` 路径排除出 Access 策略）

## C.13 防火墙配置

Cloudflare Tunnel 无需开放任何入站端口。如需最大化安全，可关闭所有入站：

```bash
ufw default deny incoming
ufw default allow outgoing
ufw allow ssh        # 保留 SSH 管理访问
ufw enable
```

## 故障排查

```bash
# Tunnel 连接状态
cloudflared tunnel info cc-control

# cloudflared 日志
journalctl -u cloudflared --since "5 min ago"

# cc-control 是否在监听
ss -tlnp | grep 18080

# agent 连接日志
journalctl -u cc-agent --since "5 min ago"

# 从服务器本地测试 cc-control
curl -sS http://127.0.0.1:18080/api/healthz
```

---

相关文档：
- Part 1（直连部署）：`01-direct-http.md`
- Part 2（TLS 部署）：`02-tls.md`
- Part 3（运维手册）：`03-operations.md`
