# API 使用说明（无需 UI）

本文档说明如何不依赖 `cc-control` 的前端 UI，通过 HTTP + WebSocket 直接控制会话。

## 概览

- **REST API**：用于查询服务器、创建/停止会话、拉取事件。
- **WebSocket API (`/ws/client`)**：用于附加会话、发送终端输入、审批动作、接收终端输出与事件。
- 结论：可以完全绕过 UI；但“发送命令/审批”目前是 **WS**，不是纯 REST。

## 鉴权

UI Token（默认 `admin-dev-token`）可通过两种方式传递：

- HTTP：`Authorization: Bearer <token>`
- WebSocket：`ws://host:port/ws/client?token=<token>`（也支持 Authorization header）

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

### 4) 创建会话

- `POST /api/sessions`
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
- 请求体（可空）：

```json
{
  "grace_ms": 4000,
  "kill_after_ms": 9000
}
```

### 6) 查询会话事件

- `GET /api/sessions/{session_id}/events`
- 返回 `events`，包括 `approval_needed` 与 resolved 状态。

---

## WebSocket API（客户端）

连接：

- `ws://127.0.0.1:18080/ws/client?token=<UI_TOKEN>`

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

#### `resize`

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
5. 收到 `event.kind=approval_needed` 后，发送 `action.kind=approve`。  
6. 监听 `session_update.awaiting_approval=false` 作为审批完成信号。  

---

## 可直接使用的脚本

仓库内已有现成脚本（无需 UI）：

- `scripts/cc-agent/test-e2e-approve-click-fix-case.sh`

它会自动执行以下步骤：启动 `cc-control + cc-agent`、创建会话、发送  
`create file approve_click_fix_case\r`、触发并完成 approve、校验状态收敛。
