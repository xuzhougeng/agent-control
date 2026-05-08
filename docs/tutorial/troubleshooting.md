# Troubleshooting

排查 cc-agent / cc-control / cc-web 常见问题。

## cc-agent 启动失败

### `flag provided but not defined: -allowed-roots`

`-allowed-roots` 不是 CLI flag，只能通过 JSON config 设置。改用：

```json
{ "allowed_roots": ["/var/log", "/etc/nginx"] }
```

或者直接去掉 `-allowed-roots` 启动（默认无路径限制，受文件系统权限保护）。

### `llm provider: anthropic: api key required`

没设 API key。检查：

```bash
echo $CC_AGENT_API_KEY        # 应该是 sk-xxx 之类
ls -la ~/.cc-agent-key        # 确保文件存在 + 600 权限
cat ~/.cc-agent-key           # 内容应该就是一行 key
```

启动时：

```bash
CC_AGENT_API_KEY="$(cat ~/.cc-agent-key)" ./cc-agent ...
```

### `cwd is required`

没传 `-cwd`，且当前目录拿不到。明确指定：

```bash
./cc-agent -cwd /var/log ...
```

## 注册到 cc-control 失败

### `duplicate server_id "xxx": already connected`

旧的 cc-agent 进程还连着 cc-control，cc-control 拒绝同 server_id 的第二条连接。

```bash
# 杀掉所有 cc-agent
sudo pkill -9 -f cc-agent

# cc-control 端会在 -offline-after-sec 秒后清理（默认 30s），再重启
sleep 5
sudo systemctl restart cc-agent
```

### `websocket: bad handshake`

通常是 token / URL / 网络问题：

```bash
# 1. URL 格式
ws://host:18180/ws/agent     # 直连
wss://host:443/ws/agent      # 经 nginx TLS

# 2. token 是否正确
curl -sS https://your-control/api/healthz   # 控制面在线
curl -sS -X GET https://your-control/api/servers \
  -H "Authorization: Bearer $UI_TOKEN"     # token 能用
```

### `protocol version mismatch`

cc-agent 和 cc-control 版本差太多。升级两边到同一大版本（v0.7.x 内互兼容）：

```bash
sudo systemctl stop cc-agent cc-control
# 都升到 v0.7.3
sudo systemctl start cc-control cc-agent
```

## chat_in 后没反应

### 模型确实在思考

DeepSeek `deepseek-v4-pro` thinking 模式单 turn 可能 1-3 分钟。看进程：

```bash
ps -ef | grep cc-agent
# 确认进程还活着，CPU 应该在 0%（等 LLM）
```

journalctl 里应该有 `agent turn start` 但还没 `agent turn done`。耐心等。

### LLM 卡住

如果超过 5 分钟仍无响应，可能是 LLM API 卡了。查 cc-agent 的 TCP 连接：

```bash
ss -tnp | grep cc-agent
# 应该至少有一条到 LLM provider 的 ESTAB 连接
```

如果没有连出去：DNS / 防火墙问题。直接 curl 测：

```bash
curl -sS --max-time 10 https://api.deepseek.com -o /dev/null -w "%{http_code}\n"
# 应该返回 401 或 200
```

### cc-control 502

如果你部署了 nginx 反代，可能 nginx 把 WS 连接超时关了。

```nginx
location /ws/ {
    proxy_pass http://127.0.0.1:18180;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_read_timeout 86400s;   # ← 关键，至少几小时
    proxy_send_timeout 86400s;
}
```

## 审批闸不工作

### 模型跑了 destructive 命令但没弹审批

可能是命令没匹配到 dangerous regex。看 cc-control 日志：

```bash
journalctl -u cc-control -f | grep -i "approval\|dangerous"
```

如果模型用了 plain `rm <file>`（v0.7.0 漏过，v0.7.1+ 已捕获）：升级到 v0.7.1+。

如果用了别的方式（`find ... -delete` / `mv to /dev/null`）：当前 regex 库不全。手动加规则到 `cc-agent/internal/tools/approval.go` 的 `dangerousMatchers`，或者直接用 `-deny-destructive` 一刀切。

### UI 上看不到 Pending Approval 卡片

1. 看 `WS: connected` 状态
2. 浏览器 hard reload (`Ctrl+Shift+R`) — ES module 缓存激进
3. cc-control 日志看 `approval_needed` 是否被广播：
   ```bash
   journalctl -u cc-control -f | grep approval_needed
   ```
4. 自己 attach 到 session 查 events：
   ```bash
   curl -sS http://localhost:18180/api/sessions \
     -H "Authorization: Bearer $UI_TOKEN" | jq
   ```

### Approve 点了但 cc-agent 没收到

cc-agent 加诊断日志：升级到 v0.7.1+，日志里会有：

```
INFO approval_decision received request_id=ar_xxx approved=true
     approver_attached=true pending_found=true
```

如果 `approver_attached=false` → main.go 没注入 RemoteApprover（检查是否被 `-deny-destructive` 或 `-full-permission` 覆盖）。

如果 `pending_found=false` → request_id 不匹配。可能是 timeout 已经清掉了 pending map。看 `-approval-timeout` 设短了？

## UI 问题

### 浏览器 cache

cc-web 用 ES module 静态导入，浏览器缓存非常激进。改了代码 / 升级 cc-control 后看不到效果：

1. **第一招**：硬刷新 `Ctrl+Shift+R`（Mac: `Cmd+Shift+R`）
2. **第二招**：开 DevTools → Network → 勾 `Disable cache` → 普通刷新
3. **第三招**：清浏览器站点缓存（Application → Storage → Clear site data）

### iOS 上看不到 server

iOS 客户端 v0.6.x 的 SessionEvent 模型没有 `agent_request_id` 字段，但应该不影响 server 列表。检查：

1. iOS App 的 `Settings` → `Server base URL` 是不是对的（含 `https://`，不带尾 `/`）
2. UI Token 是不是从同一个 cc-control 签的
3. iOS App 拉新版本（`git pull` + Xcode 重 build）

### Windows 应用启动崩溃

看事件查看器 → Windows Logs → Application。常见：

- 缺少 .NET 8 runtime → 装 [Microsoft .NET Desktop Runtime 8.x](https://dotnet.microsoft.com/download)
- 缺少 WinUI 3 SDK → 装 [Windows App SDK runtime](https://learn.microsoft.com/en-us/windows/apps/windows-app-sdk/downloads)

## 性能 / 资源

### cc-agent 内存涨

每个 chat session 持久化 message 历史。无 `-memory` 时所有都在内存里。session 多了内存会慢慢涨。

解决：

- 配 `-memory /var/lib/cc-agent/sessions.db`（SQLite 持久化，内存里只缓存最近的）
- 定期重启 cc-agent
- 长期方案：等 v0.7.x 之后的 memory compaction（roadmap 上）

### DeepSeek 偶尔 503

DeepSeek 高峰期 (UTC 02:00 - 04:00 / CST 10:00 - 12:00) 容易 503。cc-agent 当前没有自动重试，模型 turn 直接失败。

绕过：

- 改用 `claude-sonnet-4-6`（稳定但贵）
- 或本地 Ollama qwen2.5:32b 兜底
- 或忍着，等几分钟自己恢复

## 日志在哪

| 组件 | systemd | 命令行 |
|---|---|---|
| cc-control | `journalctl -u cc-control -f` | stdout/stderr |
| cc-agent | `journalctl -u cc-agent -f` | stdout/stderr |
| cc-control 审计 | `tail -f /var/lib/cc-control/audit.jsonl \| jq` | 同 |
| cc-agent session 历史 | sqlite3 `~/.cc-agent/sessions.db` | 同 |

audit.jsonl 关键 kind：

| kind | 触发条件 |
|---|---|
| `create_session` | UI 创建会话 |
| `chat_in` | 用户发消息 |
| `chat_out` | agent 输出 |
| `approval_needed` | 触发审批 |
| `action_approve` / `action_reject` | 用户决定（含 agent_request_id） |

## 没解决？

- 提 issue：<https://github.com/xuzhougeng/agent-control/issues>
- 附上：cc-agent + cc-control 版本（`./cc-agent -version`）、journalctl 完整日志、复现步骤
