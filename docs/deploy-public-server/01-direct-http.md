# 公网服务器部署（Part 1）：方案 A 直连（无 TLS）

> 适用：测试、内部网络、快速验证。  
> 若你要上生产或公网长期运行，请改用 Part 2 的 TLS 方案。

## 前置条件

- 公网服务器可被访问（示例 IP：`1.2.3.4`）
- 具备 `sudo` 权限
- 已准备 `cc-control`、`cc-agent` 源码

## A.1 编译与上传

```bash
# 编译
cd cc-control
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cc-control ./cmd/cc-control
cd ../cc-agent
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cc-agent ./cmd/cc-agent

# 上传 control 到公网服务器
scp cc-control root@1.2.3.4:/opt/cc-control/
scp -r ../ui root@1.2.3.4:/opt/cc-control/ui
```

## A.2 生成 Admin Token

```bash
ADMIN_TOKEN=$(openssl rand -hex 32)
echo "ADMIN_TOKEN=$ADMIN_TOKEN"
```

## A.3 公网服务器启动 cc-control

关键点：监听公网地址 `0.0.0.0:18080`。

```bash
/opt/cc-control/cc-control \
  -addr 0.0.0.0:18080 \
  -ui-dir /opt/cc-control/ui \
  -admin-token "$ADMIN_TOKEN" \
  -audit-path /opt/cc-control/audit.jsonl \
  -offline-after-sec 30
```

可选：如需 token 持久化，增加 `-token-db /opt/cc-control/tokens.db`（或在 `.env` 设置 `TOKEN_DB=/opt/cc-control/tokens.db`）。未设置时 token 仅内存，重启需重新签发。

Systemd（可选）`/etc/systemd/system/cc-control.service`：

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
  -addr 0.0.0.0:18080 \
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

可选：若需 Pending Approvals 自动识别，增加 `-enable-prompt-detection`。

## A.3.1 创建 Tenant Token（Admin API）

```bash
# Tenant token（返回 tenant_id）
curl -X POST http://1.2.3.4:18080/admin/tokens \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"type":"tenant"}'
```

## A.3.2 创建 UI/Agent Token（Tenant API）

```bash
# 使用 tenant token 生成 UI + Agent（默认 role=owner）
curl -X POST http://1.2.3.4:18080/tenant/tokens \
  -H "Authorization: Bearer <tenant-token>" \
  -H "Content-Type: application/json" \
  -d '{"role":"owner"}'
```

说明：每次调用会撤销该租户旧的 UI/Agent token，请同步更新浏览器和 agent 的配置。

放行端口：

```bash
ufw allow 18080/tcp
ufw enable
```

## A.4 内网机器启动 cc-agent

```bash
/opt/cc-agent/cc-agent \
  -control-url ws://1.2.3.4:18080/ws/agent \
  -agent-token "<agent-token>" \
  -server-id srv-gpu-01 \
  -allow-root /home/deploy/repos \
  -claude-path /path/to/ai-cli
```

Systemd（可选）`/etc/systemd/system/cc-agent.service`：

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
  -control-url ws://1.2.3.4:18080/ws/agent \
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

## A.5 验证

```bash
# agent 侧
journalctl -u cc-agent -f

# 浏览器
http://1.2.3.4:18080
```

使用 UI token 登录。

## A.6 安全注意事项

无 TLS 代表 token 与终端数据明文传输，仅建议测试环境使用。  
可用防火墙限制来源 IP：

```bash
ufw allow from 203.0.113.50 to any port 18080
ufw allow from 198.51.100.10 to any port 18080
ufw deny 18080/tcp
```

---

下一步：
- 上线部署看 Part 2：`02-tls.md`
- 升级/排障看 Part 3：`03-operations.md`
