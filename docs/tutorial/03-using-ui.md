# 03 · 使用指南

cc-web、iOS、Windows 三端都接到同一个 cc-control，**操作流程基本一致**。本篇以 cc-web（浏览器）为主线讲解，iOS / Windows 差异在末尾备注。

## 登录

1. 打开 cc-control 地址（`http://localhost:18180/` 或你的生产 URL）
2. 顶部有 `WS: disconnected` 状态条 + `UI Token` 输入框
3. 把 UI Token 贴进去 → `Save` → 状态变 `WS: connected` ✓

> Token 存在浏览器 localStorage，下次自动恢复。

## 选服务器 + 创建会话

左侧 `Workspace Tools`（折叠的话点 Expand）→ `Servers` 列表：

```
○ ops-prod-01  [cc-agent]  [online]
   DESKTOP-BE2AE9A
○ srv-claude-old           [online]
   nginx-host
```

- **紫色 `cc-agent` 徽章** = 新自研 agent（v0.7.0+）
- **没徽章** = cc-proxy（包外部 Claude Code 等）

点一台 → 出现 `Create Session` 表单：

| 字段 | 含义 |
|---|---|
| `cwd` | 会话工作目录（cc-agent 的 bash 工具就在这里跑）|
| `session id (optional UUID)` | 留空自动生成；填一个 UUID 可让多个客户端共享同一个会话 |
| `env` | 注入到 chat worker 的环境变量（cc-agent 不读，cc-proxy 才读）|

点 `Create` → 会话创建，URL 自动加上 `?session_id=...` 方便分享。

> 如果 server 是 cc-agent 类型，session_type 默认是 `chat`（cc-agent 不支持 PTY）。

## 会话视图

页面布局：

```
┌──────────┬────────────────────────────┬──────────────┐
│ Sessions │  Chat · 8536aa5c           │ Current      │
│          │  ─────────────────────     │ Session      │
│ [+]      │  > 用户消息                 │ - server     │
│ ops-...  │  ▶ tool_use (progress)     │ - cwd        │
│          │  ✓ tool_result (progress)  │              │
│          │  助手回复（final）          │ Pending      │
│          │  ─────────────────────     │ Approvals    │
│          │  Type a message…   [Send]  │              │
└──────────┴────────────────────────────┴──────────────┘
```

中央是 chat（cc-agent 的对话）。右侧 `Pending Approvals` 是关键 —— destructive 命令在这里弹卡片。

## 步级流式说明

cc-agent 处理一条用户消息时，**每个工具步骤都立刻推一个进度气泡**到 UI（不等整段答案）：

```
你> 看看这台机器的内核版本和负载

[淡色气泡 progress]  ▶ bash {command=uname -r}
[淡色气泡 progress]  ✓ exit_code: 0  --- stdout --- 6.6.87.2-microsoft-standard-WSL2
[淡色气泡 progress]  ▶ bash {command=uptime}
[淡色气泡 progress]  ✓ exit_code: 0  --- 19:00:21 up 2:48, load average: 0.4, 0.45, 0.41
[实色气泡 final]      Kernel **6.6.87.2** (WSL2). Load 0.4 / 0.45 / 0.41 (1m / 5m / 15m), 系统负载很低。
```

最终气泡下方的 `INTERMEDIATE STEPS` 区块也会列出所有 tool 步骤的总结。

## 审批流程（cc-agent 专属）

当模型决定跑 destructive 命令时（`rm -rf` / `mkfs` / `systemctl stop` / ...）：

1. cc-agent 检测到 dangerous pattern → 通过 cc-control 发 `approval_request`
2. 右侧 `Pending Approvals` 卡片立即弹出：

   ```
   8536aa5c [cc-agent] @ ops-prod-01
   instance ad4aef30
   [recursive rm] rm -rf /var/log/old/*.log
   [ ✓ Approve ]  [ ✕ Reject ]
   ```

3. **点 Approve** → cc-control 把决定发回 cc-agent → bash 真正执行
4. **点 Reject** → 模型收到 `DENIED by operator (reason: recursive rm)` 字符串，会自然降级到更安全的方式
5. **不点（超时）** → 等 `-approval-timeout` 时间（默认 5min，可配 30s ~ 几小时）→ 自动拒绝

紫色 `cc-agent` 徽章告诉你这是新 agent 触发的审批；没徽章的是旧 cc-proxy 的 PTY 提示。

## 多会话切换

左侧 `Sessions` 列表展示当前 tenant 下所有 session：

```
[+ create]
[refresh]

● 8536aa5c  chat   running   ← 当前选中
○ a258550e  chat   running
○ f7a91234  pty    running
○ 22b39ee1  chat   exited
```

点任意一个切换。每个 session 的历史从 cc-control 拉回（如果有 `-memory` 配置则连同 cc-agent 持久化的也一起）。

## 切换 Chat / Terminal 模式

如果选中的 server 同时跑 cc-proxy（PTY）+ cc-agent（chat），单个会话可以在两种模式之间切换（共享 session_id）。点中间的 `Chat / Terminal` 切换标签。

> cc-agent 节点只支持 chat 模式。Terminal 标签会显示 `pty mode not supported`。

## Pending Approvals 详解

右侧的卡片有几种状态：

| 视觉 | 含义 |
|---|---|
| 黄色边框 + `[✓ Approve] [✕ Reject]` | 等待决定 |
| 紫色 `cc-agent` 徽章 | 来源是 cc-agent destructive 命令 |
| 无徽章 | 来源是 cc-proxy 的 PTY 提示（如 Claude Code 的 `Allow this command? [y/N]`）|
| 卡片消失 | 已决定 / 超时 / session 被销毁 |

**点 Approve 之前会看到完整命令** —— 不是抽象描述，而是即将真的跑的那一行 shell。**先看一眼再点**。

## 通知

右侧 `Notifications` 区块显示来自 cc-agent 的主动推送（v0.6.x 加的功能）。可以在 cc-agent 节点上发：

```bash
curl -X POST http://your-control:18180/api/notifications \
  -H "Authorization: Bearer $UI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"Backup done","message":"successful in 4m23s","level":"success","source":"cron"}'
```

通知会推到所有当前 attached 的 UI（含 iOS / Windows）。

## iOS / Windows 客户端差异

| 功能 | cc-web | iOS | Win |
|---|---|---|---|
| Chat 会话 | ✓ | ✓ | ✓ |
| Pending Approvals | ✓ | ✓ | ✓ |
| `cc-agent` 徽章（server 列表）| ✓ | ✓ | ✓ |
| `cc-agent` 徽章（审批卡片）| ✓ | ✓（v0.7.2+）| ✓（v0.7.2+）|
| Terminal/PTY 模式 | ✓ | ✓ macOS / iOS | ✗（不支持 PTY）|
| Skill 管理（`:reflect`）| 暂只 CLI | 暂只 CLI | 暂只 CLI |

iOS 端要在 [App 设置](../../app/AgentControlMac/Sources/iOS/Views/SettingsView.swift) 里配 `Server base URL` + `UI Token`，然后操作流程和 cc-web 一致。

## 下一步

- 配 LLM provider → [04-Provider 配置](04-providers.md)
- 报错排查 → [troubleshooting](troubleshooting.md)
- 写 skill / 用 `:reflect` → [`cc-agent/README.md` 的 Skills 节](../../cc-agent/README.md#skills)
