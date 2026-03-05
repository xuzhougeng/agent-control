# Web E2E 设计说明

本文描述 Web E2E 的目录结构、覆盖范围与设计要点。运行步骤与调试见 [如何跑 Web E2E 测试](how-to-test-e2e.md)。

Web E2E 当前由五套入口组成：

1. 默认回归：`npm run test:web:e2e`
2. Notification 专项回归：`npm run test:web:e2e:notification` 或 `bash scripts/test-web-notification-e2e.sh`
3. 移动端样式回归：`npm run test:web:mobile`
4. 移动端 fake Claude Terminal 回归：`npm run test:web:mobile:terminal`
5. 真实 Claude 烟测：`npm run test:web:e2e:real-claude`

## 目录

- `tests/web-e2e/playwright.config.mjs`
- `tests/web-e2e/playwright.mobile.config.mjs`
- `tests/web-e2e/playwright.mobile.terminal.config.mjs`
- `tests/web-e2e/run-harness.sh`
- `tests/web-e2e/run-harness-echo.sh`
- `tests/web-e2e/specs/workspace.spec.js`
- `tests/web-e2e/specs/notification.spec.js`
- `tests/web-e2e/specs/mobile-scroll.spec.js`
- `tests/web-e2e/specs/mobile-terminal-fake.spec.js`
- `tests/web-e2e/specs/real-claude.spec.js`
- `tests/web-e2e/fixtures/fake-claude.py`
- `tests/web-e2e/fixtures/xterm-stub.js`
- `tests/web-e2e/fixtures/xterm-stub.css`

## 覆盖内容

### `workspace.spec.js`（默认回归）

1. Chat 视图创建统一 session 并发送消息
2. `Chat -> Terminal -> Chat` 切换，且实例历史不会无限增长（断言为 2 条）
3. Terminal 视图可接入预置外部 Claude session
4. 左右抽屉支持垂直拖拽
5. 移动端抽屉开合互斥，点击 backdrop 可统一关闭
6. Terminal `copy` 事件会把选中文本写入 clipboard
7. `/chat` 兼容路径重定向到统一 workspace（`view=chat`）

### `notification.spec.js`（通知专项）

1. 创建一个 PTY session
2. 调用 `POST /api/notifications` 主动发送通知
3. 断言右侧通知列表出现该条通知
4. 断言页面 toast 出现该条通知
5. 保存包含通知的截图到 `test-results/`

### `mobile-scroll.spec.js`（移动端样式回归）

1. Chat 气泡中的代码块在移动端可横向滚动
2. 代码块不会因 `word-break: break-all` 被强行断词
3. Markdown 表格会被正确渲染为 table 结构
4. Markdown data URL 图片可正常渲染
5. 消息加载后输入栏和 `Send` 按钮仍在可视区内

### `mobile-terminal-fake.spec.js`（移动端 fake Claude Terminal）

1. 在 mobile 视口创建 Terminal session
2. 终端可收到 fake Claude 启动输出（`Started session`）
3. 通过 `window.__CC_E2E__.sendTerminalInput(...)` 发送输入并回显

### `real-claude.spec.js`（真实 Claude 烟测）

1. Terminal 创建 session 并发送 `hi`
2. 切到 Chat 后发送 `hi`，确认会话未进入 `exited/error`
3. 再切回 Terminal 发送 `say hi`
4. 关键步骤落盘截图，便于排查真实链路问题

运行方式与端口、环境变量用法见 [如何跑 Web E2E 测试](how-to-test-e2e.md)。

## 设计说明

- `run-harness.sh` 会临时编译并拉起 `cc-control`、`cc-agent`、`cc-chat-claude`，并等待 `/api/servers` 报告 `srv-e2e` 在线。
- 默认模式 `CC_WEB_E2E_CLAUDE_MODE=fake` 使用 `fake-claude.py`，覆盖 `--session-id`/`--resume`、会话冲突、会话不存在等分支。
- `run-harness-echo.sh` 使用 `cc-chat-echo`，用于移动端代码块滚动和输入栏布局回归，不依赖 Claude CLI。
- `workspace.spec.js` 默认拦截 CDN 的 `xterm.js/xterm.css` 并注入本地 stub，降低外网依赖。
- `mobile-terminal-fake.spec.js` 在 mobile 视口下走 fake Claude PTY 链路，并使用 xterm stub 消除外网依赖。
- `real-claude.spec.js` 仅在 `CC_WEB_E2E_CLAUDE_MODE=real` 时执行；可选 `CC_WEB_E2E_XTERM_STUB=1` 强制启用 xterm stub 辅助排查渲染问题。

## 关键环境变量

- `CC_WEB_E2E_PORT`
  - 默认回归默认 `18110`，notification 专项脚本默认 `18114`，移动端样式回归默认 `18112`，移动端 terminal 回归默认 `18113`
- `CC_WEB_E2E_UI_TOKEN` / `CC_WEB_E2E_AGENT_TOKEN`
  - control 与 agent 的测试 token
- `CC_WEB_E2E_SERVER_ID`
  - agent 上报的 server id，默认 `srv-e2e`
- `CC_WEB_E2E_ALLOW_ROOT`
  - agent 允许工作目录根路径
- `CC_WEB_E2E_PRESEEDED_SESSION_ID`
  - 预置外部会话 id（默认 `11111111-1111-4111-8111-111111111111`）
- `CC_WEB_E2E_CLAUDE_MODE`
  - `fake` 或 `real`（默认 `fake`）
- `CC_WEB_E2E_CLAUDE_PATH` / `CC_WEB_E2E_CLAUDE_HOME`
  - 真实 Claude 模式下 CLI 路径与 `HOME`
- `CC_WEB_E2E_SCREENSHOT`
  - 默认回归截图策略（默认 `on`）
- `CC_WEB_E2E_VIDEO`
  - 录屏策略（默认 `off`；real-claude npm 脚本里默认 `on`）
- `CC_WEB_E2E_XTERM_STUB`
  - real-claude 用例中是否启用 xterm stub（`1` 为启用）
