# API 使用说明（无需 UI）

本文档说明如何不依赖 `cc-control` 的前端 UI，通过 HTTP + WebSocket 直接控制会话。

## 概览

- **REST API**：用于查询服务器、创建/停止会话、拉取事件。
- **WebSocket API (`/ws/client`)**：用于附加会话、发送终端输入、审批动作、接收终端输出与事件。
- 结论：可以完全绕过 UI；但“发送命令/审批”目前是 **WS**，不是纯 REST。

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
> Tenant Token 仅用于租户自助签发接口（见下文），不用于 UI/WS。
> 所有请求均按 token 所属 `tenant_id` 隔离，跨租户资源会返回 `not found`。

---

## Admin API（Token 管理）

Base URL：`http://127.0.0.1:18080`

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

---

## Tenant API（自助签发 UI/Agent Token）

Base URL：`http://127.0.0.1:18080`

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
  "server_id": "srv-local",
  "cwd": "/Users/you/Documents",
  "env": {"CC_PROFILE": "dev"},
  "cols": 120,
  "rows": 30,
  "resume_id": "optional"
}
```

- 成功：`201`，返回 `session` 对象（含 `session_id`）。

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

### 7) 删除会话

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

### 服务端 -> 客户端

- `debug_probe`：调试探针，可忽略。
- `attach_ok`：attach 成功确认。
- `term_out`：终端输出（`data_b64`）。
- `event`：业务事件，重点是 `approval_needed`。
- `session_update`：会话状态更新（含 `awaiting_approval`、`pending_event_id`）。
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
