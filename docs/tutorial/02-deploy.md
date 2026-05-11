# 02 · 生产部署

把 cc-agent 部署到一台真实 Linux 服务器（生产或 staging），让它通过远程 cc-control 接受 chat 任务。

> 假设你已经有一个**外网可达的 cc-control 实例**（自己跑或团队共享）。如果还没有，
> 先看 [公网部署总览](../deploy-public-server.md) 把 cc-control 起来再回到这一篇。

> 适用于 Linux/amd64 + Linux/arm64。其他系统（macOS、Windows server）方法类似。

## 前置准备

### 1. 在 cc-control 签一个 agent token

使用**生产 cc-control 的 admin token** 走两步签发（先 tenant，再 agent/UI）：

```bash
TENANT=$(curl -s -X POST https://your-control.example.com/admin/tokens \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" -d '{"type":"tenant"}' \
  | jq -r .token)

curl -s -X POST https://your-control.example.com/tenant/tokens \
  -H "Authorization: Bearer $TENANT" \
  -H "Content-Type: application/json" -d '{"role":"owner"}'
```

记下返回的 `agent.token`。

### 2. LLM API key

```bash
echo 'sk-xxxxxxxxxxxxxxxx' | sudo tee /etc/cc-agent/api-key > /dev/null
sudo chown root:root /etc/cc-agent/api-key
sudo chmod 600 /etc/cc-agent/api-key
```

## 安装 cc-agent

```bash
# 二进制
sudo curl -L -o /usr/local/bin/cc-agent \
  https://github.com/xuzhougeng/agent-control/releases/latest/download/cc-agent-linux-amd64
sudo chmod +x /usr/local/bin/cc-agent

# 数据目录
sudo useradd -r -s /usr/sbin/nologin -d /var/lib/cc-agent cc-agent
sudo install -d -o cc-agent -g cc-agent /var/lib/cc-agent /etc/cc-agent /etc/cc-agent/skills.d
```

## systemd unit

`/etc/systemd/system/cc-agent.service`：

```ini
[Unit]
Description=cc-agent (server-ops AI agent)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=cc-agent
Group=cc-agent

# 运维要让 agent 操作的根目录。bash/read/write 限制在这些目录下。
WorkingDirectory=/var/log

Environment=CC_AGENT_PROVIDER=deepseek
Environment=CC_AGENT_BASE_URL=https://api.deepseek.com
Environment=CC_AGENT_MEMORY_PATH=/var/lib/cc-agent/sessions.db
Environment=CC_AGENT_SKILLS_DIR=/etc/cc-agent/skills.d
Environment=CC_AGENT_APPROVAL_TIMEOUT=30m

# 关键：从文件读 API key，不要写在命令行（ps 里能看见）
ExecStartPre=/bin/bash -c 'test -r /etc/cc-agent/api-key'
ExecStart=/bin/bash -c 'CC_AGENT_API_KEY="$(cat /etc/cc-agent/api-key)" \
  exec /usr/local/bin/cc-agent \
    -model deepseek-chat \
    -cwd /var/log \
    -control-url wss://your-control.example.com/ws/agent \
    -agent-token <REPLACE_WITH_AGENT_TOKEN> \
    -server-id ops-prod-01'

Restart=on-failure
RestartSec=5

# 安全加固
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=/var/log /var/lib/cc-agent /etc/cc-agent
ProtectHome=true
PrivateTmp=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictNamespaces=true
RestrictRealtime=true
RestrictSUIDSGID=true
LockPersonality=true

[Install]
WantedBy=multi-user.target
```

启动：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now cc-agent
sudo systemctl status cc-agent
journalctl -u cc-agent -f
```

成功的话日志里会看到：

```
[approval] UI-routed approver: timeout=30m0s
[control] registering with wss://your-control.example.com/ws/agent as ops-prod-01
INFO cc-control connected
INFO cc-control register_ok server_id=ops-prod-01
running as daemon (no REPL); send SIGINT/SIGTERM to stop.
```

## 关键配置项解释

### `-cwd` 与 `-allowed-roots`

bash 工具被严格限制在 `-cwd`。但 read/write 工具可以指定 `allowed_roots`（一组允许的根路径）做 path containment。

通过 JSON 配置：

```bash
# /etc/cc-agent/config.json
sudo tee /etc/cc-agent/config.json <<EOF
{
  "provider": "deepseek",
  "model": "deepseek-chat",
  "base_url": "https://api.deepseek.com",
  "cwd": "/var/log",
  "allowed_roots": ["/var/log", "/etc/nginx", "/var/lib/redis"],
  "max_iterations": 12,
  "max_tokens": 4096,
  "memory_path": "/var/lib/cc-agent/sessions.db",
  "skills_dir": "/etc/cc-agent/skills.d",
  "approval_timeout": "30m"
}
EOF
sudo chown cc-agent:cc-agent /etc/cc-agent/config.json
```

然后 unit 里把 ExecStart 改成 `-config /etc/cc-agent/config.json`，这样不用塞 env。

### `-approval-timeout`

| 场景 | 推荐 |
|---|---|
| 运维实时盯着 chat | `30s` ~ `1m` |
| 普通工作时间 | `5m` ~ `10m` |
| 夜班 / 异步 | `30m` ~ `1h` |
| 跨时区 / 离线 | `4h` ~ `8h` |

### Permission 模式选择

| 场景 | 用什么 |
|---|---|
| 真实生产，运维点头才执行 destructive | **默认**（`-control-url` + 不加其他 flag） |
| 全自动定时巡检 + 不允许任何破坏性 | `-deny-destructive` |
| 临时受信脚本，全跑 | `-full-permission`（**慎用**，仅短期） |

`-full-permission` 优先于 `-deny-destructive` 优先于默认 cc-control 桥。

## 多服务器部署

每台 server 跑一个 cc-agent，**共用一个 cc-control**：

```bash
# server A
-server-id ops-app-01 -cwd /var/log/app

# server B
-server-id ops-db-01 -cwd /var/log/postgresql

# server C  
-server-id ops-cache-01 -cwd /var/log/redis
```

UI 上左侧 `Servers` 列表会同时显示三台，每台带紫色 `cc-agent` 徽章。运维选哪台就在哪台上下 chat。

## 监控与运维

### 查看 cc-agent 健康

```bash
# 进程状态
systemctl status cc-agent

# 实时日志
journalctl -u cc-agent -f

# 看是否在线
curl -s https://your-control.example.com/api/servers \
  -H "Authorization: Bearer $UI_TOKEN" | jq '.servers[] | {server_id, status}'
```

### 审计日志

cc-control 把所有关键事件写到 `audit.jsonl`：

```bash
tail -f /var/lib/cc-control/audit.jsonl | jq
```

重要 kind：

- `create_session` — 谁开了一个 chat
- `chat_in` — 谁发了什么
- `chat_out` — agent 输出了什么
- `approval_needed` — 触发审批
- `action_approve` / `action_reject` — 谁做了什么决定（含 agent_request_id）

### 查 destructive 命令历史

```bash
grep '"kind":"approval_needed"' /var/lib/cc-control/audit.jsonl | jq
grep '"kind":"action_approve"\|"kind":"action_reject"' /var/lib/cc-control/audit.jsonl | jq
```

## 升级流程

```bash
# 1. 拉新 binary
sudo curl -L -o /tmp/cc-agent.new \
  https://github.com/xuzhougeng/agent-control/releases/download/<NEW_VERSION>/cc-agent-linux-amd64
sudo chmod +x /tmp/cc-agent.new

# 2. 替换 + 重启
sudo systemctl stop cc-agent
sudo mv /tmp/cc-agent.new /usr/local/bin/cc-agent
sudo systemctl start cc-agent
journalctl -u cc-agent -n 20
```

cc-control / cc-proxy 流程一样。多版本互兼容（向后兼容是设计目标）。

## 常见配置示例

### A. 日志巡检（read-only）

```ini
Environment=CC_AGENT_APPROVAL_TIMEOUT=1h
ExecStart=... -cwd /var/log -deny-destructive ...
```

### B. nginx 故障排查（含重载）

```ini
Environment=CC_AGENT_APPROVAL_TIMEOUT=1m
# 不加 -deny-destructive，nginx -s reload 会触发审批，运维确认才执行
ExecStart=... -cwd /etc/nginx ...
```

### C. 备份脚本类（dd / mkfs 都不该跑）

```ini
ExecStart=... -cwd /backup -deny-destructive ...
```

`-deny-destructive` 模式下 dd/mkfs 直接被拒，模型自然降级。

## 下一步

- UI 怎么用 → [03-使用指南](03-using-ui.md)
- 切 LLM provider → [04-Provider 配置](04-providers.md)
- 报错排查 → [troubleshooting](troubleshooting.md)
