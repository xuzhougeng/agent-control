# 公网服务器部署（Part 2）：TLS（域名 / 自签名）

> 适用：生产部署、跨公网长期运行。  
> 本文包含：
> - 方案 B：域名 + Let's Encrypt（推荐）
> - 方案 B'：无域名 + 自签名 TLS

## 方案 B：Nginx + TLS（域名）

### B.1 编译与上传

```bash
cd cc-control
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cc-control ./cmd/cc-control

scp cc-control root@1.2.3.4:/opt/cc-control/
scp -r ../ui root@1.2.3.4:/opt/cc-control/ui
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
  -ui-dir /opt/cc-control/ui \
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
```

```bash
useradd -r -s /sbin/nologin cc
chown -R cc:cc /opt/cc-control
chmod 600 /opt/cc-control/.env
systemctl daemon-reload
systemctl enable --now cc-control
```

可选：增加 `-enable-prompt-detection`。

### B.3.1 创建 UI/Agent Token（Admin API）

```bash
# UI token（owner），返回 tenant_id
curl -X POST https://cc.example.com/admin/tokens \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"type":"ui","role":"owner"}'

# Agent token（同 tenant_id）
curl -X POST https://cc.example.com/admin/tokens \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"type":"agent","tenant_id":"<tenant_id>"}'
```

### B.4 配置 Nginx + Let's Encrypt

```bash
apt install -y nginx certbot python3-certbot-nginx
certbot --nginx -d cc.example.com
```

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

### B.5 部署 cc-agent

```bash
cd cc-agent
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cc-agent ./cmd/cc-agent
scp cc-agent user@internal-host:/opt/cc-agent/
```

`/etc/systemd/system/cc-agent.service`：

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
AGENT_TOKEN=<agent-token-from-admin-api>
SERVER_ID=srv-gpu-01
```

```bash
chmod 600 /opt/cc-agent/.env
systemctl daemon-reload
systemctl enable --now cc-agent
```

### B.6 验证

```bash
journalctl -u cc-agent -f
```

浏览器访问 `https://cc.example.com`，使用 UI token 登录。

---

## 方案 B'：无域名 + 自签名 TLS

### B'.1 生成自签名证书（含 IP SAN）

将 `1.2.3.4` 替换为你的公网 IP。

```bash
mkdir -p /opt/cc-control/tls
cd /opt/cc-control/tls

cat > openssl.cnf << 'EOF'
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
EOF

openssl req -x509 -nodes -days 3650 -newkey rsa:2048 \
  -keyout key.pem -out cert.pem -config openssl.cnf -extensions v3_req
```

### B'.2 cc-control 与方案 B 相同

按 B.1～B.3 配置 `cc-control`（仍使用 `-admin-token`）。

### B'.3 Nginx 使用自签名证书

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

### B'.4 cc-agent 需跳过证书校验

```bash
/opt/cc-agent/cc-agent \
  -control-url wss://1.2.3.4/ws/agent \
  -tls-skip-verify \
  -agent-token "<agent-token>" \
  -server-id srv-gpu-01 \
  -allow-root /home/deploy/repos \
  -claude-path /path/to/ai-cli
```

Systemd 场景可在 `.env` 中设置 `TLS_SKIP_VERIFY=1`。

### B'.5 验证

- 浏览器访问 `https://1.2.3.4`，先接受证书警告，再用 UI token 登录
- `journalctl -u cc-agent -f` 观察 agent 连接状态

---

下一步：
- 客户端接入、安全加固、批量部署、升级迁移请看 Part 3：`03-operations.md`
