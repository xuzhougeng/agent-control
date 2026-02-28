# How To Test Web E2E

这份文档面向本地开发，重点说明怎么运行和调试 `tests/web-e2e/` 下的浏览器端到端测试。

更偏设计说明的内容见 [docs/web-e2e.md](/home/xzg/project/agent-control/docs/web-e2e.md)。

## 前提

需要本机具备：

- `go`
- `node` / `npm`
- `python3`

首次运行还需要安装 Playwright 浏览器和系统依赖：

```bash
npm install
npx playwright install
sudo npx playwright install-deps chromium
```

## 直接运行

在仓库根目录执行：

```bash
npm run test:web:e2e
```

这条命令会：

1. 启动一个本地 harness
2. 在临时目录里启动 `cc-control`、`cc-agent`、`cc-chat-claude`
3. 用 `tests/web-e2e/fixtures/fake-claude.py` 替代真实 Claude CLI
4. 运行 Playwright 用例
5. 为每条用例输出 screenshot，默认保存在 `test-results/`

默认模式是 `fake`。如果要单独跑真实 Claude smoke，用下面这条：

```bash
npm run test:web:e2e:real-claude
```

这条命令会把 harness 切到 `CC_WEB_E2E_CLAUDE_MODE=real`，并默认使用：

- `CC_WEB_E2E_CLAUDE_PATH=/home/xzg/.local/bin/claude`
- `CC_WEB_E2E_CLAUDE_HOME=$HOME`

真实 Claude smoke 当前只覆盖一条窄用例：

1. 在 Terminal 视图创建 session
2. 向 PTY 发送 `hi`
3. 等待 `10s`
4. 切到 Chat 并点击 `Switch to Chat`
5. 再切回 Terminal 并点击 `Switch to Terminal`

这条 smoke 还会为每个关键步骤单独落盘 screenshot，包括：

- workspace ready
- session created
- sent hi
- after 10s wait
- before/after switch to chat
- before/after switch back to terminal

如果你需要覆盖不同账号或不同 Claude 安装路径，可以显式传：

```bash
CC_WEB_E2E_CLAUDE_PATH=/custom/path/to/claude \
CC_WEB_E2E_CLAUDE_HOME=/custom/home \
npm run test:web:e2e:real-claude
```

## 指定端口

默认端口是 `18110`。如果本机端口冲突，可以改：

```bash
CC_WEB_E2E_PORT=18120 npm run test:web:e2e
```

## Screenshot 策略

默认会为每条用例保存整页 screenshot，方便直接检查页面状态，输出目录是：

- `test-results/`

如果你只想在失败时保留 screenshot：

```bash
CC_WEB_E2E_SCREENSHOT=only-on-failure npm run test:web:e2e
```

如果你临时不想生成 screenshot：

```bash
CC_WEB_E2E_SCREENSHOT=off npm run test:web:e2e
```

## 当前覆盖

当前这批用例主要覆盖：

1. Chat 视图创建统一 session 并发送消息
2. `Chat -> Terminal -> Chat` 模式切换
3. 实例列表不会因为来回切换而膨胀
4. 预先存在的外部 Claude session 能通过 PTY 接入
5. `/chat` 兼容入口会重定向到统一 workspace

## 常用调试点

### 1. 看 harness 入口

入口脚本是：

- `tests/web-e2e/run-harness.sh`

这里负责：

- 编译本地二进制
- 起 control / agent
- 预置外部 session marker
- 把 Web UI 指向本地 control

### 2. 看 fake Claude 行为

替身脚本是：

- `tests/web-e2e/fixtures/fake-claude.py`

它模拟了：

- `--session-id`
- `--resume`
- `Session ID ... is already in use`
- `No conversation found with session ID ...`

如果回归问题和 Claude session 恢复有关，优先看这里。

### 3. 只跑单个 spec

```bash
npx playwright test tests/web-e2e/specs/workspace.spec.js --config=tests/web-e2e/playwright.config.mjs
```

### 4. 打开 Playwright UI

```bash
npx playwright test --ui --config=tests/web-e2e/playwright.config.mjs
```

## 相关环境变量

- `CC_WEB_E2E_PORT`
  - harness 对外暴露的 control 端口
- `CC_WEB_E2E_UI_TOKEN`
  - Web UI 使用的 token
- `CC_WEB_E2E_AGENT_TOKEN`
  - agent 使用的 token
- `CC_WEB_E2E_SERVER_ID`
  - 测试 agent 的 server id
- `CC_WEB_E2E_ALLOW_ROOT`
  - agent 的 `allow-root`
- `CC_WEB_E2E_PRESEEDED_SESSION_ID`
  - 用于模拟“服务器上已存在 Claude session”的会话 id
- `CC_WEB_E2E_SCREENSHOT`
  - screenshot 策略，支持 `on` / `only-on-failure` / `off`
- `CC_WEB_E2E_CLAUDE_MODE`
  - `fake` 或 `real`，默认 `fake`
- `CC_WEB_E2E_CLAUDE_PATH`
  - real Claude 模式下的 Claude CLI 路径，默认 `/home/xzg/.local/bin/claude`
- `CC_WEB_E2E_CLAUDE_HOME`
  - real Claude 模式下 agent/chat worker 使用的 `HOME`

## 常见问题

### 缺少 Chromium 系统依赖

如果 Playwright 报浏览器缺少系统库，重新执行：

```bash
sudo npx playwright install-deps chromium
```

### 端口占用

如果 `18110` 已被占用，改 `CC_WEB_E2E_PORT`。

### 想验证真实 Claude，而不是 fake Claude

这套 E2E 默认目标不是验证真实 Claude，而是稳定回归 Web UI、control plane、agent 的联动行为。

如果要测真实 Claude，建议单独做 smoke 流程，不要直接替换掉当前这套回归测试。
