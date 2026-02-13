# 公网服务器部署（Part 3）：客户端接入、运维与升级

> 本文包含：
> - 客户端连接（macOS App / Browser）
> - 安全加固清单
> - 多 Agent 批量部署
> - 旧版本破坏性升级（legacy -> admin-token）
> - 常见故障排查

## 1. 客户端连接

### 1.1 macOS 原生客户端

`app/AgentControlMac` 可连接任意已部署的 `cc-control`。

1. 打开应用，按 `Cmd+,` 进入 Settings
2. 填写：
   - Base URL
     - 直连：`http://公网IP:18080`
     - 域名 TLS：`https://cc.example.com`
     - 自签名 TLS：`https://公网IP`（勾选 Skip TLS verification）
   - UI Token：由 Tenant API 创建（需 tenant token）
3. 点击 Save & Reconnect

### 1.2 浏览器

- 直连：`http://公网IP:18080`
- 域名 TLS：`https://cc.example.com`
- 自签名 TLS：`https://公网IP`（先手动信任证书）

登录均使用 UI token。

## 2. 安全加固清单

| 项目 | 方案 A (无 TLS) | 方案 B (域名 TLS) | 方案 B' (自签名 TLS) |
|------|----------------|-------------------|----------------------|
| 传输加密 | 无，token 明文 | TLS 加密 | TLS 加密 |
| Token | 强随机，防火墙限源 IP | 强随机即可 | 强随机即可 |
| allow-root | 严格限制到项目目录 | 同左 | 同左 |
| 运行用户 | 非 root | 同左 | 同左 |
| 端口暴露 | 防火墙限源 IP | 仅 80/443 | 仅 443 |
| UI 访问限制 | 防火墙限源 IP | Nginx IP 白名单 / Basic Auth | 同左 |
| 日志审计 | `audit.jsonl` 定期归档 | 同左 | 同左 |
| 证书 | — | Let's Encrypt | 自签名，agent 用 `-tls-skip-verify` |

## 3. 多 Agent 批量部署（可选）

```bash
for host in gpu01 gpu02 gpu03; do
  scp cc-agent $host:/opt/cc-agent/
  ssh $host "cat > /opt/cc-agent/.env << EOF
AGENT_TOKEN=<agent-token-from-tenant-api>
SERVER_ID=srv-$host
EOF
chmod 600 /opt/cc-agent/.env
systemctl daemon-reload
systemctl enable --now cc-agent"
done
```

## 4. 旧版本破坏性升级（legacy token -> admin token）

适用于旧部署使用 `-ui-token/-agent-token`，并允许短暂中断。

### 4.1 备份

```bash
TS=$(date +%Y%m%d-%H%M%S)
mkdir -p ~/cc-upgrade-backup/$TS
sudo cp /opt/cc-control/cc-control ~/cc-upgrade-backup/$TS/cc-control.bin.bak
sudo cp /etc/systemd/system/cc-control.service ~/cc-upgrade-backup/$TS/cc-control.service.bak
sudo cp /opt/cc-control/.env ~/cc-upgrade-backup/$TS/cc-control.env.bak
```

### 4.2 切到 `-admin-token` 启动

`/etc/systemd/system/cc-control.service` 核心参数：

```ini
ExecStart=/opt/cc-control/cc-control \
  -addr 127.0.0.1:18080 \
  -ui-dir /opt/cc-control/ui \
  -admin-token ${ADMIN_TOKEN} \
  -audit-path /opt/cc-control/audit.jsonl \
  -offline-after-sec 30
```

`/opt/cc-control/.env`：

```bash
ADMIN_TOKEN=<your-admin-token>
```

重启：

```bash
sudo systemctl daemon-reload
sudo systemctl reset-failed cc-control || true
sudo systemctl restart cc-control
curl -sS http://127.0.0.1:18080/api/healthz
```

### 4.3 签发新 token

```bash
# 先创建 tenant token
curl -X POST https://<control-host>/admin/tokens \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"type":"tenant"}'

# 再用 tenant token 生成 UI + Agent token
curl -X POST https://<control-host>/tenant/tokens \
  -H "Authorization: Bearer <tenant-token>" \
  -H "Content-Type: application/json" \
  -d '{"role":"owner"}'
```

说明：
- token 默认内存态；如需跨重启保留，可启动时配置 `-token-db <path>` 或 `TOKEN_DB=<path>`（SQLite）。
- 切换后 `servers` 为空通常是 agent 仍使用旧 token。

### 4.4 逐台重启 agent

```bash
/opt/cc-agent/cc-agent \
  -control-url wss://<control-host>/ws/agent \
  -agent-token "<new-agent-token>" \
  -server-id srv-gpu-01 \
  -allow-root /home/deploy/repos \
  -claude-path /path/to/ai-cli
```

自签名 TLS 场景加 `-tls-skip-verify`。

### 4.5 回滚（可选）

```bash
TS=<backup-ts>
sudo cp ~/cc-upgrade-backup/$TS/cc-control.bin.bak /opt/cc-control/cc-control
sudo cp ~/cc-upgrade-backup/$TS/cc-control.service.bak /etc/systemd/system/cc-control.service
sudo cp ~/cc-upgrade-backup/$TS/cc-control.env.bak /opt/cc-control/.env
sudo systemctl daemon-reload
sudo systemctl restart cc-control
```

## 5. 故障排查

```bash
# control 监听
ss -tlnp | grep 18080

# agent 连接日志
journalctl -u cc-agent --since "5 min ago"

# nginx 代理
curl -v https://cc.example.com/api/servers

# 防火墙
ufw status
```

---

相关文档：
- Part 1（直连部署）：`01-direct-http.md`
- Part 2（TLS 部署）：`02-tls.md`
