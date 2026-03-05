# API 使用说明（无需 UI）

本文档说明如何不依赖 `cc-control` 的前端 UI，通过 HTTP + WebSocket 直接控制会话。

## 概览

- **REST API**：用于查询服务器、创建/停止会话、拉取事件、发送跨端提醒通知。
- **WebSocket API (`/ws/client`)**：用于附加会话、发送终端输入、审批动作、接收终端输出与事件。
- 结论：可以完全绕过 UI；“发送终端命令/审批”依然是 **WS**，但提醒通知可通过 REST 主动推送。

## 鉴权

UI Token 可通过两种方式传递：

- HTTP：`Authorization: Bearer <token>`
- WebSocket：`ws://host:port/ws/client?token=<token>`（也支持 Authorization header）
  - 如果 Control Plane 走 TLS，请使用 `wss://host/ws/client?token=<token>`

UI Token 具备角色权限：

- `viewer`：只读
- `operator`：可创建/停止会话，发送终端输入与动作
- `owner`：包含 `operator` 权限 + 删除会话

> Admin Token 仅用于管理接口（见下文），不用于 UI/WS。
> Tenant Token 可用于租户自助签发接口与租户通知接口（见下文），不用于 UI/WS。
> 所有请求均按 token 所属 `tenant_id` 隔离，跨租户资源会返回 `not found`。

---

## Admin API（Token 管理）

Base URL：`http://127.0.0.1:18080`

### 0) 校验 token

- `GET /admin/verify` 或 `POST /admin/verify`
- Header：`Authorization: Bearer <ADMIN_TOKEN>`
- 响应：

```json
{
  "ok": true,
  "token_id": "uuid",
  "type": "admin",
  "tenant_id": ""
}
```

### 1) 创建 token

- `POST /admin/tokens`
- Header：`Authorization: Bearer <ADMIN_TOKEN>`
- 请求体：

```json
{
  "type": "ui|agent|tenant",
  "tenant_id": "optional",
  "role": "viewer|operator|owner (ui only)",
  "name": "optional"
}
```

- 响应（仅返回一次明文 token）：

```json
{
  "token": "plain-text",
  "token_id": "uuid",
  "tenant_id": "uuid",
  "type": "ui|agent|tenant",
  "role": "viewer|operator|owner",
  "created_at_ms": 1730000000000
}
```

### 2) 撤销 token

- `POST /admin/tokens/{token_id}/revoke`
- Header：`Authorization: Bearer <ADMIN_TOKEN>`
- 响应：

```json
{"ok": true}
```

### 3) 列出 token

- `GET /admin/tokens?tenant_id=...`
- Header：`Authorization: Bearer <ADMIN_TOKEN>`
- 响应：

```json
{
  "tokens": [
    {
      "token_id": "uuid",
      "tenant_id": "uuid",
      "type": "ui|agent|tenant|admin",
      "role": "viewer|operator|owner",
      "created_at_ms": 1730000000000,
      "revoked": false,
      "name": "optional"
    }
  ]
}
```

### 4) 查询服务器（跨租户）

- `GET /admin/servers`
- 可选过滤：`GET /admin/servers?tenant_id=...`
- Header：`Authorization: Bearer <ADMIN_TOKEN>`
- 响应：

```json
{
  "servers": [
    {
      "server_id": "srv-local",
      "hostname": "host",
      "status": "online",
      "tenant_id": "uuid",
      "os": "linux",
      "arch": "amd64",
      "version": "1.0.0",
      "last_seen_ms": 1730000000000
    }
  ]
}
```

> 说明：`tenant_id` 为空时返回所有租户的服务器；指定时仅返回该租户的服务器。

### 5) 查询会话（跨租户）

- `GET /admin/sessions`
- 可选过滤：`GET /admin/sessions?tenant_id=...&server_id=...`
- Header：`Authorization: Bearer <ADMIN_TOKEN>`
- 响应：

```json
{
  "sessions": [
    {
      "session_id": "uuid",
      "server_id": "srv-local",
      "tenant_id": "uuid",
      "status": "running",
      "cwd": "/path/to/dir"
    }
  ]
}
```

> 说明：`tenant_id` 和 `server_id` 均为可选过滤参数，均为空时返回全部会话。

### 6) 停止会话（跨租户）

- `POST /admin/sessions/{session_id}/stop`
- Header：`Authorization: Bearer <ADMIN_TOKEN>`
- 请求体（可空）：

```json
{
  "grace_ms": 4000,
  "kill_after_ms": 9000
}
```

- 响应：

```json
{"ok": true}
```

> 说明：Admin 可停止任意租户的会话，无需指定 `tenant_id`。

---

## Tenant API（自助签发 UI/Agent Token）

Base URL：`http://127.0.0.1:18080`

### 0) 校验 token

- `GET /tenant/verify` 或 `POST /tenant/verify`
- Header：`Authorization: Bearer <TENANT_TOKEN>`
- 响应：

```json
{
  "ok": true,
  "token_id": "uuid",
  "type": "tenant",
  "tenant_id": "uuid"
}
```

### 1) 生成 UI + Agent token（自动撤销旧 token）

- `POST /tenant/tokens`
- Header：`Authorization: Bearer <TENANT_TOKEN>`
- 请求体（可选）：

```json
{
  "tenant_id": "optional (must match tenant token)",
  "role": "viewer|operator|owner (ui role, default owner)",
  "ui_name": "optional",
  "agent_name": "optional"
}
```

- 响应（仅返回一次明文 token）：

```json
{
  "tenant_id": "uuid",
  "revoked_count": 2,
  "ui": {
    "token": "plain-text",
    "token_id": "uuid",
    "type": "ui",
    "role": "owner",
    "created_at_ms": 1730000000000
  },
  "agent": {
    "token": "plain-text",
    "token_id": "uuid",
    "type": "agent",
    "created_at_ms": 1730000000000
  }
}
```

> 说明：每次调用会撤销该 `tenant_id` 现有的 UI/Agent token，请同步更新浏览器和 agent 的配置。

### 2) 发布/查询通知（Tenant Token）

- `POST /tenant/notifications`
- `GET /tenant/notifications?limit=20`
- Header：`Authorization: Bearer <TENANT_TOKEN>`
- 作用：允许自动化脚本直接发送提醒，不需要 UI Token。
- 请求体与 `/api/notifications` 基本一致（`message` 必填，支持 `title/level/source/session_id/server_id`）。
- 返回：

```json
{
  "ok": true,
  "notification": {
    "notification_id": "uuid",
    "kind": "notification",
    "tenant_id": "uuid",
    "message": "backup done",
    "level": "info",
    "source": "external",
    "ts_ms": 1730000000000
  }
}
```

---

## REST API

Base URL：`http://127.0.0.1:18080`

### 1) 健康检查

- `GET /api/healthz`
- 响应：

```json
{"ok": true}
```

### 2) 查询服务器

- `GET /api/servers`
- 角色要求：`viewer` 及以上
- 响应：

```json
{
  "servers": [
    {
      "server_id": "srv-local",
      "hostname": "host",
      "status": "online"
    }
  ]
}
```

### 3) 查询会话

- `GET /api/sessions`
- 可选过滤：`GET /api/sessions?server_id=srv-local`
- 角色要求：`viewer` 及以上

### 4) 创建会话

- `POST /api/sessions`
- 角色要求：`operator` 及以上
- 请求体：

```json
{
  "session_id": "optional-uuid",
  "server_id": "srv-local",
  "session_type": "pty",
  "cwd": "/Users/you/Documents",
  "env": {"CC_PROFILE": "dev"},
  "cols": 120,
  "rows": 30
}
```

- `session_id`：可选 UUID。未传时服务端自动生成；传入时会校验合法性并拒绝重复值（`409`）。
- `session_id` 同时作为逻辑会话标识和 Claude conversation ID 复用。
- `session_type`：可选，`"pty"` 或 `"chat"`。`chat` 类型不需要 `cols/rows`。
- 未传 `session_type` 时：
  - 非 Windows server 默认 `pty`
  - Windows server 默认 `chat`
- Windows server 暂不支持 `session_type=pty`；若显式传 `pty` 会返回 `400`，错误文本：`PTY is not supported on Windows yet; use session_type=chat`。
- 成功：`201`，返回 `session` 对象（含 `session_id`、`session_type`、`active_instance_id`）。

补充行为：

- 当 `session_type=pty` 时，agent 会优先按以下规则决定 Claude CLI 参数：
  - 若本机已存在 `~/.claude/session-env/<session_id>`，使用 `--resume <session_id>`；
  - 否则使用 `--session-id <session_id>` 创建/继续该逻辑会话。
- 这使得你可以传入一个已经在服务器上用 `claude` 启动过的 UUID 型 `session_id`，让 PTY 直接接入该 conversation。

### 5) 停止会话

- `POST /api/sessions/{session_id}/stop`
- 角色要求：`operator` 及以上
- 请求体（可空）：

```json
{
  "grace_ms": 4000,
  "kill_after_ms": 9000
}
```

### 6) 查询会话事件

- `GET /api/sessions/{session_id}/events`
- 角色要求：`viewer` 及以上
- 返回 `events`。如果启用了 `cc-control -enable-prompt-detection`，可能会出现 `approval_needed`（以及对应的 resolved 状态）；否则通常为空或仅包含非 approval 类事件（如未来扩展）。

### 7) 查询实例列表

- `GET /api/sessions/{session_id}/instances`
- 角色要求：`viewer` 及以上
- 返回该逻辑会话下各模式的运行实例槽位。
- 返回：

```json
{
  "instances": [
    {
      "instance_id": "uuid",
      "session_id": "uuid",
      "session_type": "pty|chat",
      "status": "starting|running|stopping|exited|error",
      "created_at_ms": 1730000000000
    }
  ]
}
```

说明：

- 同一逻辑 `session_id` 下通常最多看到一条 `pty` 和一条 `chat` 实例槽位。
- 反复 `Chat -> PTY -> Chat` 切换时，系统会复用对应模式的实例槽位，而不是无限创建新实例。

### 8) 查询聊天历史

- `GET /api/sessions/{session_id}/chat`
- 角色要求：`viewer` 及以上
- 仅适用于 `session_type=chat` 的会话。
- 返回：

```json
{
  "messages": [
    {
      "message_id": "uuid",
      "session_id": "uuid",
      "instance_id": "uuid",
      "role": "user|assistant",
      "content": "message text",
      "meta": {"operations": ["optional step"]},
      "ts_ms": 1730000000000
    }
  ]
}
```

#### Chat Worker（Claude Code 无头模式）

`session_type=chat` 的推荐实现是使用无头 worker 调用 Claude Code。无头模式不支持交互式审批菜单（1/2/3），因此应在会话创建时通过参数/策略一次性限定能力（例如 `dontAsk` + allowed tools）。

> 当前推荐先以 Multi-Chat 路径落地（例如 `cc-chat-echo`），GPT 模式可后续单独扩展。

可用的环境变量（由 `StartSessionRequest.env` 透传到 worker）：

- `CC_CLAUDE_CMD`：Claude Code 可执行文件路径（默认 `claude`）
- `CC_CLAUDE_PERMISSION_MODE`：默认 `dontAsk`
- `CC_CLAUDE_ALLOWED_TOOLS` / `CC_CLAUDE_DISALLOWED_TOOLS`
- `CC_CLAUDE_MODEL` / `CC_CLAUDE_EFFORT`
- `CC_CLAUDE_SYSTEM_PROMPT` / `CC_CLAUDE_APPEND_SYSTEM_PROMPT`
- `CC_CLAUDE_ADD_DIR`：逗号分隔的允许目录（会转成多个 `--add-dir`）
- `CC_CLAUDE_BETAS`
- `CC_CLAUDE_TIMEOUT_MS`
- `CC_CLAUDE_PROFILE_FILE`（从文件加载个性化提示词）
- `CC_CLAUDE_INJECT_RUNTIME_CONTEXT`（是否注入运行时上下文，默认开启）

### 9) 发布通知（UI Token）

- `POST /api/notifications`
- 角色要求：`operator` 及以上
- 请求体：

```json
{
  "title": "Deploy Completed",
  "message": "prod-us-east-1 finished in 8m23s",
  "level": "info",
  "source": "ci",
  "session_id": "optional-session-id",
  "server_id": "optional-server-id"
}
```

字段说明：

- `message` 必填
- `level` 可选：`info|success|warning|error`（默认 `info`）
- `session_id` 可选；传入后会校验租户归属，并自动补齐 `server_id`（如果未提供）
- `source` 可选，建议写任务来源（如 `cron`, `ci`, `backup`）

- 成功：`201`

```json
{
  "ok": true,
  "notification": {
    "notification_id": "uuid",
    "kind": "notification",
    "tenant_id": "uuid",
    "level": "success",
    "title": "Deploy Completed",
    "message": "prod-us-east-1 finished in 8m23s",
    "source": "ci",
    "actor": "ui:<token_id>",
    "ts_ms": 1730000000000
  }
}
```

### 10) 查询通知（UI Token）

- `GET /api/notifications?limit=20`
- 角色要求：`viewer` 及以上
- 返回最近通知（默认 20，最多 100）：

```json
{
  "notifications": [
    {
      "notification_id": "uuid",
      "kind": "notification",
      "tenant_id": "uuid",
      "level": "info",
      "title": "Build Done",
      "message": "all tests passed",
      "source": "ci",
      "ts_ms": 1730000000000
    }
  ]
}
```

### 11) 切换会话模式

- `POST /api/sessions/{session_id}/switch`
- 角色要求：`operator` 及以上
- 请求体：

```json
{
  "session_type": "pty",
  "env": {"CC_PROFILE": "dev"},
  "cols": 120,
  "rows": 30
}
```

说明：

- `session_id` 不变，`active_instance_id` 可能切换到该模式对应的实例槽位。
- 服务端流程是严格串行的：先停掉当前 active instance，等待退出，再启动目标模式实例。
- 对 PTY 而言：
  - 若当前逻辑会话已存在 chat history，控制面会优先要求 PTY 用 `--resume <session_id>`；
  - 若该逻辑会话还没有真实 conversation，控制面不会盲目要求 `resume`，避免 `no conversation found with session ID ...`。

- 成功：`200`，返回更新后的 `session` 对象。

### 12) 删除会话

- `DELETE /api/sessions/{session_id}`
- 角色要求：`owner`
- 删除语义等价于 `Stop + Deletion`：
  - 若会话处于 `starting/running/stopping`，服务端会先发送 stop，再立即删除会话记录；
  - 若会话已结束/错误，直接删除会话记录。
- 成功返回：`200 {"ok": true}`

---

## WebSocket API（客户端）

连接：

- `ws://127.0.0.1:18080/ws/client?token=<UI_TOKEN>`
- TLS 场景：`wss://cc.example.com/ws/client?token=<UI_TOKEN>`

统一消息封包（Envelope）：

```json
{
  "type": "xxx",
  "server_id": "optional",
  "session_id": "optional",
  "seq": 123,
  "ts_ms": 1730000000000,
  "data": {...},
  "data_b64": "optional"
}
```

### 客户端 -> 服务端

#### `attach`

附加到一个 session，接收快照和后续输出。

```json
{
  "type": "attach",
  "data": {
    "session_id": "SESSION_ID",
    "since_seq": 0
  }
}
```

#### `term_in`

向终端写入输入（Base64）。
角色要求：`operator` 及以上

```json
{
  "type": "term_in",
  "session_id": "SESSION_ID",
  "data_b64": "Y3JlYXRlIGZpbGUgYS50eHQN"
}
```

> 例如 `create file a.txt\r` 需先编码为 Base64。

#### `action`

审批/拒绝/停止动作。
角色要求：`operator` 及以上

```json
{
  "type": "action",
  "session_id": "SESSION_ID",
  "data": {
    "kind": "approve"
  }
}
```

`kind` 支持：

- `approve`
- `reject`
- `stop`

`event_id` 可选；即使传入旧值，服务端会按当前 pending approval 处理。  
注意：`approve/reject` 仅在 `awaiting_approval=true`（通常意味着启用了 `-enable-prompt-detection` 且命中了 prompt）时有效，否则会返回 `no pending approval`。

#### `resize`
角色要求：`operator` 及以上

```json
{
  "type": "resize",
  "session_id": "SESSION_ID",
  "data": {"cols": 120, "rows": 30}
}
```

#### `chat_in`

向聊天会话发送一条用户消息。仅适用于 `session_type=chat` 的会话。
角色要求：`operator` 及以上

```json
{
  "type": "chat_in",
  "session_id": "SESSION_ID",
  "data": {
    "content": "describe image",
    "content_parts": [
      {"type": "text", "text": "describe image"},
      {
        "type": "image",
        "source": {
          "type": "base64",
          "media_type": "image/png",
          "data": "BASE64_DATA"
        }
      }
    ]
  }
}
```

说明：

- `content_parts` 为可选，支持 `text` 与 `image(base64)`。
- 向后兼容：仅传 `content` 仍可用。
- `image.source` 当前仅支持 `type=base64` 且 `media_type` 需为 `image/*`。

### 服务端 -> 客户端

- `debug_probe`：调试探针，可忽略。
- `attach_ok`：attach 成功确认。
- `term_out`：终端输出（`data_b64`）。
- `chat_msg`：聊天消息（仅 `session_type=chat`），`data` 包含 `{message_id, role, content, meta?, ts_ms}`。
- `event`：业务事件，包含：
  - `kind=approval_needed`：审批提示
  - `kind=notification`：外部主动提醒（来自 `/api/notifications` 或 `/tenant/notifications`）
- `session_update`：会话状态更新（含 `awaiting_approval`、`pending_event_id`、`session_type`）。
- `error`：错误消息，`data.message` 为错误文本。

---

## 无 UI 自动化最小流程

1. `POST /api/sessions` 创建会话，拿 `session_id`。  
2. 连接 `/ws/client`。  
3. 发送 `attach` 到该 session。  
4. 发送 `term_in`（例如 `create file approve_click_fix_case\r`）。  
5. 如果启用了 `-enable-prompt-detection` 且收到 `event.kind=approval_needed`，再发送 `action.kind=approve`（或 `reject`）。  
6. 否则：直接通过 `term_in` 手动发送按键（例如 Enter / y / n / Esc 等）完成交互。

---

## 可直接使用的脚本

仓库内已有现成脚本（无需 UI）：

- `scripts/cc-agent/test-e2e-approve-click-fix-case.sh`

它会自动执行以下步骤：启动 `cc-control + cc-agent`、创建会话、发送  
`create file approve_click_fix_case\r`、触发并完成 approve、校验状态收敛。
