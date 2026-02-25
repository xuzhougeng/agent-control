# 公网服务器部署（Part 2）：TLS（域名 / 自签名）

适用：生产部署、跨公网长期运行。  
本文按常用顺序：自签名（B）→ 域名 + Let's Encrypt（B'）→ Cloudflare 托管域名 + Origin CA（B''）。

## 方案 B：无域名 + 自签名 TLS（完整流程）

### B.1 编译与上传

```bash
# 编译
cd cc-control
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cc-control ./cmd/cc-control
cd ../cc-agent
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cc-agent ./cmd/cc-agent

# 上传 control 到公网服务器
scp cc-control root@1.2.3.4:/opt/cc-control/
scp -r ../cc-web root@1.2.3.4:/opt/cc-control/cc-web
```

### B.2 生成 Admin Token

```bash
ADMIN_TOKEN=$(openssl rand -hex 32)
echo "ADMIN_TOKEN=$ADMIN_TOKEN"
```

### B.3 配置并启动 cc-control

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

可选：如需 token 持久化，可设置 `TOKEN_DB`（或在 `ExecStart` 中添加 `-token-db /opt/cc-control/tokens.db`）。未设置时 token 仅内存，重启需重新签发。

```bash
useradd -r -s /sbin/nologin cc
chown -R cc:cc /opt/cc-control
chmod 600 /opt/cc-control/.env
systemctl daemon-reload
systemctl enable --now cc-control
```

可选：增加 `-enable-prompt-detection`。

### B.4 生成自签名证书（含 IP SAN）

将 `1.2.3.4` 替换为你的公网 IP。

```bash
mkdir -p /opt/cc-control/tls
cd /opt/cc-control/tls

cat > openssl.cnf << 'EOF2'
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no
[req_distinguished_name]
CN = cc-control
[v3_req]
subjectAltName = @alt
[alt]
IP.1 = 1.2.3.4
EOF2

openssl req -x509 -nodes -days 3650 -newkey rsa:2048 \
  -keyout key.pem -out cert.pem -config openssl.cnf -extensions v3_req
```

### B.5 Nginx 使用自签名证书

`/etc/nginx/conf.d/cc.conf`：

```nginx
server {
    listen 443 ssl http2 default_server;
    listen [::]:443 ssl http2 default_server;
    server_name _;

    ssl_certificate     /opt/cc-control/tls/cert.pem;
    ssl_certificate_key /opt/cc-control/tls/key.pem;

    location /ws/ {
        proxy_pass http://127.0.0.1:18080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }

    location / {
        proxy_pass http://127.0.0.1:18080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

```bash
nginx -t && systemctl reload nginx
ufw allow 443/tcp
ufw enable
```

### B.6 创建 Tenant Token（Admin API）

```bash
# Tenant token（返回 tenant_id）
curl -k -X POST https://1.2.3.4/admin/tokens \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"type":"tenant"}'
```

### B.7 创建 UI/Agent Token（Tenant API）

```bash
# 使用 tenant token 生成 UI + Agent（默认 role=owner）
curl -k -X POST https://1.2.3.4/tenant/tokens \
  -H "Authorization: Bearer <tenant-token>" \
  -H "Content-Type: application/json" \
  -d '{"role":"owner"}'
```

说明：每次调用会撤销该租户旧的 UI/Agent token，请同步更新浏览器和 agent 的配置。

### B.8 部署 cc-agent（启用 -tls-skip-verify）

```bash
# 上传 agent（二选一：用你实际上传方式）
scp cc-agent user@internal-host:/opt/cc-agent/

# 手动启动
/opt/cc-agent/cc-agent \
  -control-url wss://1.2.3.4/ws/agent \
  -tls-skip-verify \
  -agent-token "<agent-token>" \
  -server-id srv-gpu-01 \
  -allow-root /home/deploy/repos \
  -claude-path /path/to/ai-cli
```

Systemd 场景（示意）：

```ini
ExecStart=/opt/cc-agent/cc-agent \
  -control-url wss://1.2.3.4/ws/agent \
  -tls-skip-verify \
  -agent-token ${AGENT_TOKEN} \
  -server-id ${SERVER_ID} \
  -allow-root /home/deploy/repos \
  -claude-path /path/to/ai-cli
```

也可在 `.env` 里设置 `TLS_SKIP_VERIFY=1`。

### B.9 验证

- 浏览器访问 `https://1.2.3.4`，先接受证书警告，再用 UI token 登录。
- `journalctl -u cc-agent -f` 观察 agent 连接状态。

---

## 方案 B'：域名 + Let's Encrypt（由 B 改造）

`cc-control` 与 token 流程可直接沿用 B 的 B.1～B.7。  
主要改动是证书来源、Nginx `server_name`、以及 agent 去掉 `-tls-skip-verify`。

### B'.0 B -> B' 逐项替换清单

| 项目 | 方案 B（无域名/自签名） | 方案 B'（域名/Let's Encrypt） |
|------|--------------------------|------------------------------|
| 访问入口 | `https://<公网IP>` | `https://<domain>` |
| 证书来源 | `/opt/cc-control/tls/cert.pem`（自签名） | `/etc/letsencrypt/live/<domain>/fullchain.pem` |
| Nginx `server_name` | `_` | 你的域名（如 `cc.example.com`） |
| 80 端口用途 | 可不开放 | 建议开放用于 ACME 验证和 80->443 跳转 |
| Agent 启动参数 | 需要 `-tls-skip-verify` | 去掉 `-tls-skip-verify` |
| 浏览器首次访问 | 需手动信任证书 | 常规受信任证书，无警告 |

### B'.1 申请证书（Let's Encrypt）

```bash
apt install -y nginx certbot python3-certbot-nginx
certbot --nginx -d cc.example.com
```

### B'.2 Nginx 配置为域名证书 + 80 跳转

`/etc/nginx/conf.d/cc.conf`：

```nginx
server {
    listen 443 ssl http2;
    server_name cc.example.com;

    ssl_certificate     /etc/letsencrypt/live/cc.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/cc.example.com/privkey.pem;

    location /ws/ {
        proxy_pass http://127.0.0.1:18080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }

    location / {
        proxy_pass http://127.0.0.1:18080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}

server {
    listen 80;
    server_name cc.example.com;
    return 301 https://$host$request_uri;
}
```

```bash
nginx -t && systemctl reload nginx
ufw allow 80/tcp
ufw allow 443/tcp
ufw enable
```

### B'.3 cc-agent 去掉 `-tls-skip-verify`

```bash
/opt/cc-agent/cc-agent \
  -control-url wss://cc.example.com/ws/agent \
  -agent-token "<agent-token>" \
  -server-id srv-gpu-01 \
  -allow-root /home/deploy/repos \
  -claude-path /path/to/ai-cli
```

### B'.4 验证

- 浏览器访问 `https://cc.example.com`，使用 UI token 登录。
- `journalctl -u cc-agent -f` 观察 agent 连接状态。

---

## 方案 B''：Cloudflare 托管域名 + Origin CA 证书

域名在 Cloudflare 托管，使用 Cloudflare 签发的 **Origin Certificate** 部署在源站，客户端与 Agent 通过 Cloudflare 边缘访问，无需 Let's Encrypt 与 `-tls-skip-verify`。

### B''.0 与 B/B' 对比

| 项目 | 方案 B'（Let's Encrypt） | 方案 B''（Cloudflare） |
|------|--------------------------|-------------------------|
| 证书来源 | Let's Encrypt（ACME） | Cloudflare Origin CA（控制台签发） |
| 证书有效期 | 90 天需续期 | 最长 15 年 |
| 80 端口 | 需开放给 ACME | 可选（若仅用 Cloudflare 代理可不必开放） |
| Agent | 不用 `-tls-skip-verify` | 不用 `-tls-skip-verify` |

### B''.1 域名托管到 Cloudflare

1. 在 [Cloudflare Dashboard](https://dash.cloudflare.com) 添加站点，按提示将域名的 NS 改为 Cloudflare 提供的 NS。
2. DNS 里添加 A 记录，例如：
   - 名称：`cc`（或 `@` 用根域）
   - 内容：你的源站公网 IP
   - 代理状态：**已代理**（橙色云）推荐，流量经 Cloudflare 再回源。

### B''.2 创建 Origin Certificate

1. 进入 **SSL/TLS → Origin Server**，点击 **Create Certificate**。
2. 选择 **Let Cloudflare generate a private key and a CSR**，有效期选 15 年。
3. 主机名填：`cc.example.com`（或你的实际域名），可勾选「Include wildcard」如 `*.example.com`。
4. 点击 **Create**，保存显示的 **Origin Certificate**（PEM）和 **Private Key**（PEM）。

### B''.3 源站安装证书

在源站服务器上：

```bash
mkdir -p /opt/cc-control/tls
# 将 Cloudflare 控制台中的 Origin Certificate 内容写入 cert.pem
# 将 Private Key 内容写入 key.pem
nano /opt/cc-control/tls/cert.pem   # 粘贴证书
nano /opt/cc-control/tls/key.pem    # 粘贴私钥
chown -R cc:cc /opt/cc-control/tls
chmod 600 /opt/cc-control/tls/key.pem
```

### B''.4 设置 SSL/TLS 模式（若使用代理）

在 Cloudflare：**SSL/TLS → Overview** 中，加密模式选 **Full** 或 **Full (Strict)**（推荐 Full Strict，因 Origin 使用 Cloudflare 签发的证书）。

### B''.5 Nginx 配置

`/etc/nginx/conf.d/cc.conf`：

```nginx
server {
    listen 443 ssl http2;
    server_name cc.example.com;

    ssl_certificate     /opt/cc-control/tls/cert.pem;
    ssl_certificate_key /opt/cc-control/tls/key.pem;

    location /ws/ {
        proxy_pass http://127.0.0.1:18080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }

    location / {
        proxy_pass http://127.0.0.1:18080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

若希望 HTTP 跳转 HTTPS，可加 80 端口的 `server` 块（同 B'.2）；若全程经 Cloudflare 代理且只开放 443，可不开放 80。

```bash
nginx -t && systemctl reload nginx
ufw allow 443/tcp
# ufw allow 80/tcp   # 仅在有 80 跳转时开放
ufw enable
```

### B''.6 cc-agent 连接

使用托管域名，无需 `-tls-skip-verify`（客户端与 Agent 均面对 Cloudflare 边缘证书）：

```bash
/opt/cc-agent/cc-agent \
  -control-url wss://cc.example.com/ws/agent \
  -agent-token "<agent-token>" \
  -server-id srv-gpu-01 \
  -allow-root /home/deploy/repos \
  -claude-path /path/to/ai-cli
```

### B''.7 验证

- 浏览器访问 `https://cc.example.com`，使用 UI token 登录。
- `journalctl -u cc-agent -f` 观察 agent 连接状态。

---

下一步：
- Cloudflare Tunnel 部署（零端口暴露）看 Part 2a：`02a-cloudflare-tunnel.md`
- 客户端接入、安全加固、批量部署、升级迁移请看 Part 3：`03-operations.md`
