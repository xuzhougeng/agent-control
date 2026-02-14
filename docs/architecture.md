# Agent Control 架构

## 系统总览

```mermaid
flowchart TB
    subgraph Clients["客户端"]
        Browser["Browser\n(xterm.js / cc-web)"]
        App["AgentControl App\n(macOS / iOS)"]
    end

    subgraph Control["控制面"]
        CC["cc-control\nREST + WebSocket"]
    end

    subgraph Agents["Agent 节点"]
        A1["cc-agent\nsrv-01"]
        A2["cc-agent\nsrv-02"]
    end

    Browser -->|"REST / WS\n(UI Token)"| CC
    App -->|"REST / WS\n(UI Token)"| CC
    CC <-->|"WS /ws/agent\n(Agent Token)"| A1
    CC <-->|"WS /ws/agent\n(Agent Token)"| A2
```

## 多用户隔离（匿名租户）

- `agent_token` 与 `ui_token` 均绑定 `tenant_id`，中心服务器只按 tenant 维度隔离，不关心真实身份。
- UI 角色：`viewer` / `operator` / `owner`。
- `admin_token` 用于生成/撤销 tenant token。
- `tenant_token` 用于该租户自助签发 UI/Agent token（每次生成会刷新旧 token）。
- token 默认内存态；可通过 `-token-db` / `TOKEN_DB` 持久化到 SQLite 以跨重启保留。

## 组件与目录

```mermaid
flowchart LR
    subgraph Repo["agent-control 仓库"]
        CC_DIR["cc-control/"]
        AGENT_DIR["cc-agent/"]
        UI_DIR["cc-web/"]
        APP_DIR["app/AgentControlMac/"]
    end

    CC_DIR -->|"控制面\nHTTP + WS"| CC_SVC["cc-control 进程"]
    AGENT_DIR -->|"每机一个\n出站 WS + PTY"| AGENT_SVC["cc-agent 进程"]
    UI_DIR -->|"静态前端"| BROWSER["浏览器"]
    APP_DIR -->|"原生客户端"| NATIVE["macOS/iOS App"]
```

## 核心数据流（创建会话与终端）

```mermaid
sequenceDiagram
    participant UI as Browser/App
    participant CP as cc-control
    participant Agent as cc-agent

    Agent->>CP: WS /ws/agent (register + heartbeat)
    CP-->>Agent: register_ok
    UI->>CP: GET /api/servers
    CP-->>UI: servers
    UI->>CP: POST /api/sessions
    CP->>Agent: start_session(session_id, cwd, env, cmd)
    Agent-->>CP: pty_out (流式)
    CP-->>UI: session 列表更新
    UI->>CP: WS /ws/client attach(session_id)
    CP-->>UI: term_out (回放 + 实时)
    UI->>CP: term_in（手动按键）/ action(approve|reject)（可选）
    CP->>Agent: pty_in("y\\n"/"n\\n"/Enter/Esc)（可选）
    UI->>CP: POST /api/sessions/{id}/stop
    CP->>Agent: stop_session
    Agent-->>CP: pty_exit
    CP-->>UI: session_update(exited)
```

> 说明：`approval_needed`/Pending Approvals 属于 **启发式 prompt detection**（`cc-control -enable-prompt-detection`），默认关闭；关闭时不会自动产生 Pending Approvals，但终端交互（`term_in`）仍可正常使用。

## 部署拓扑：直连（方案 A）

```mermaid
flowchart TB
    subgraph Public["公网服务器 1.2.3.4"]
        CC["cc-control :18080\n监听 0.0.0.0"]
    end

    subgraph Intranet["内网"]
        A1["内网机器 A\ncc-agent"]
        A2["内网机器 B\ncc-agent"]
    end

    A1 -->|"ws:// (outbound)"| CC
    A2 -->|"ws:// (outbound)"| CC
```

## 部署拓扑：Nginx + TLS（方案 B）

```mermaid
flowchart TB
    subgraph Public["公网服务器"]
        LE["Let's Encrypt"]
        Nginx["Nginx :443"]
        CC["cc-control :18080"]
        Nginx --> LE
        Nginx --> CC
    end

    subgraph Intranet["内网"]
        A1["内网机器 A\ncc-agent"]
        A2["内网机器 B\ncc-agent"]
    end

    A1 -->|"wss:// (outbound)"| Nginx
    A2 -->|"wss:// (outbound)"| Nginx
```

## 依赖与技术栈

各子项目使用的依赖库如下，便于后续开发与升级。

### 前端（cc-web/）

- **技术**：原生 HTML/CSS/JavaScript，无构建工具，无 `package.json`。
- **运行时依赖**（通过 CDN 引入，见 `cc-web/index.html`）：
  - **xterm.js** — 终端模拟（`xterm/lib/xterm.js`、`xterm/css/xterm.css`）
  - **xterm-addon-fit** — 终端自适应窗口大小（`xterm-addon-fit/lib/xterm-addon-fit.js`）

### 控制面（cc-control/）

- **语言**：Go 1.25
- **依赖**（`cc-control/go.mod`）：

| 依赖 | 用途 |
|------|------|
| `github.com/google/uuid` | 生成 session_id 等唯一标识 |
| `github.com/gorilla/websocket` | WebSocket 服务端（/ws/agent、/ws/client） |
| `modernc.org/sqlite` | SQLite 驱动（`-token-db` 持久化 token） |

### Agent 节点（cc-agent/）

- **语言**：Go 1.25
- **依赖**（`cc-agent/go.mod`）：

| 依赖 | 用途 |
|------|------|
| `github.com/creack/pty` | 创建 PTY，与 shell/子进程交互 |
| `github.com/gorilla/websocket` | 出站 WebSocket 连接控制面 |

### macOS / iOS 客户端（app/AgentControlMac/）

- **语言**：Swift，Xcode 项目 + Swift Package Manager
- **直接依赖**（SPM）：
  - **SwiftTerm**（`migueldeicaza/SwiftTerm`）— 终端渲染与输入，macOS 用 `NSViewRepresentable`，iOS 用 `UIViewRepresentable`
- **传递依赖**（`Package.resolved` 中）：
  - **swift-argument-parser** — 由 SwiftTerm 引入
