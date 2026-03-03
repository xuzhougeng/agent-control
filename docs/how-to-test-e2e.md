# 如何跑 Web E2E 测试

本文说明如何运行和调试 `tests/web-e2e/` 下的浏览器端到端测试（执行步骤、环境、调试）。测试范围与设计说明见 [Web E2E 设计说明](web-e2e.md)。

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

## 快速入口

仓库根目录下有四条常用命令：

1. 默认回归（fake Claude）  
   `npm run test:web:e2e`
2. 移动端样式回归（echo worker）  
   `npm run test:web:mobile`
3. 移动端 fake Claude Terminal 回归  
   `npm run test:web:mobile:terminal`
4. 真实 Claude 烟测  
   `npm run test:web:e2e:real-claude`

## 默认回归（fake Claude）

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

如需指定端口（默认 `18110`）：

```bash
CC_WEB_E2E_PORT=18120 npm run test:web:e2e
```

## 移动端样式回归（echo worker）

```bash
npm run test:web:mobile
```

这条命令使用 `tests/web-e2e/playwright.mobile.config.mjs`，仅执行 `mobile-scroll.spec.js`，并且：

1. 使用 `tests/web-e2e/run-harness-echo.sh`
2. 在临时目录启动 `cc-control`、`cc-agent`、`cc-chat-echo`
3. 设备配置固定为 `iPhone 14`
4. 默认端口 `18112`

如需指定端口：

```bash
CC_WEB_E2E_PORT=18122 npm run test:web:mobile
```

## 移动端 fake Claude Terminal 回归

```bash
npm run test:web:mobile:terminal
```

这条命令使用 `tests/web-e2e/playwright.mobile.terminal.config.mjs`，仅执行 `mobile-terminal-fake.spec.js`，并且：

1. 使用 `tests/web-e2e/run-harness.sh`
2. 运行 fake Claude 模式（`CC_WEB_E2E_CLAUDE_MODE=fake`）
3. 在 mobile 视口创建 Terminal session 并验证 PTY 输入回显
4. 使用 xterm stub，避免依赖外网 CDN
5. 默认端口 `18113`

如需指定端口：

```bash
CC_WEB_E2E_PORT=18123 npm run test:web:mobile:terminal
```

## 真实 Claude 烟测

```bash
npm run test:web:e2e:real-claude
```

这条命令会把 harness 切到 `CC_WEB_E2E_CLAUDE_MODE=real`，并在 npm 脚本中默认设置：

- `CC_WEB_E2E_PORT=18111`
- `CC_WEB_E2E_VIDEO=on`
- 运行 `tests/web-e2e/specs/real-claude.spec.js`

默认使用：

- `CC_WEB_E2E_CLAUDE_PATH=/home/xzg/.local/bin/claude`
- `CC_WEB_E2E_CLAUDE_HOME=$HOME`
- 真实 `xterm.js` 渲染（不默认 stub）

如果要单独覆盖 Claude 安装路径或 HOME，用：

```bash
CC_WEB_E2E_PORT=18111 \
CC_WEB_E2E_CLAUDE_PATH=/custom/path/to/claude \
CC_WEB_E2E_CLAUDE_HOME=/custom/home \
npm run test:web:e2e:real-claude
```

真实 Claude smoke 当前覆盖一条窄路径：

1. 在 Terminal 视图创建 session
2. 点击该 session，并将左侧抽屉折叠回去
3. 点击 Terminal 视图，等待 `5s`
4. 向 PTY 发送 `hi`
5. 再等待 `10s`
6. 切到 Chat 并点击 `Switch to Chat`
7. 在 Chat 再发送 `hi`
8. 校验会话不会进入 `Execution failed: exited`
9. 再等待 `10s`
10. 点击 Terminal 并点击 `Switch to Terminal`
11. 在 Terminal 再发送 `say hi`

这条 smoke 还会为每个关键步骤单独落盘 screenshot，包括：

- workspace ready
- session created
- terminal wait 5s before hi
- sent hi
- after 10s wait
- before/after switch to chat
- sent hi in chat
- chat still running
- after chat wait 10s
- before/after switch back to terminal
- sent `say hi` after switching back to terminal

### Terminal 键盘发送注意事项（real-claude）

real-claude 用例里，Terminal 输入默认走“真实键盘路径”：

1. 先点击终端区域聚焦
2. `keyboard.type(...)`
3. `Enter` 提交

只有键盘路径在限定时间内没有观测到 `term_in` 时，才回退到 `window.__CC_E2E__.sendTerminalInput(...)`。

排查“看起来输入了 hi，但没有真正发送”时，不要只看截图里的输入行；以测试日志里的这些事件为准：

- `terminal:send:*:keyboard-ok` 或 `terminal:send:*:ws-fallback-ok`
- `terminal:term-out-observed` / `terminal:final-term-out-observed`

## Screenshot 策略

默认会为每条用例保存 screenshot，输出目录：

- `test-results/`

录屏文件（当 `CC_WEB_E2E_VIDEO=on`）也会写在 `test-results/` 下对应 case 目录中，常见文件名是 `video.webm`。

只在失败时保留 screenshot：

```bash
CC_WEB_E2E_SCREENSHOT=only-on-failure npm run test:web:e2e
```

临时关闭 screenshot：

```bash
CC_WEB_E2E_SCREENSHOT=off npm run test:web:e2e
```

各 spec 的覆盖内容与设计说明见 [Web E2E 设计说明](web-e2e.md)。

## 常用调试点

### 1. 看 harness 入口

入口脚本是：

- `tests/web-e2e/run-harness.sh`
- `tests/web-e2e/run-harness-echo.sh`

前者用于默认回归/real-claude，后者用于 mobile 回归（`cc-chat-echo`）。

`run-harness.sh` 主要负责：

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

```bash
npx playwright test tests/web-e2e/specs/mobile-scroll.spec.js --config=tests/web-e2e/playwright.mobile.config.mjs
```

```bash
CC_WEB_E2E_PORT=18113 CC_WEB_E2E_CLAUDE_MODE=fake npx playwright test tests/web-e2e/specs/mobile-terminal-fake.spec.js --config=tests/web-e2e/playwright.mobile.terminal.config.mjs
```

```bash
CC_WEB_E2E_PORT=18111 CC_WEB_E2E_CLAUDE_MODE=real npx playwright test tests/web-e2e/specs/real-claude.spec.js --config=tests/web-e2e/playwright.config.mjs
```

### 4. 打开 Playwright UI（默认回归）

```bash
npx playwright test --ui --config=tests/web-e2e/playwright.config.mjs
```

常用环境变量：`CC_WEB_E2E_PORT`、`CC_WEB_E2E_SCREENSHOT`、`CC_WEB_E2E_CLAUDE_MODE`、`CC_WEB_E2E_CLAUDE_PATH`。完整列表与含义见 [Web E2E 设计说明 - 关键环境变量](web-e2e.md#关键环境变量)。

## 常见问题

### 缺少 Chromium 系统依赖

如果 Playwright 报浏览器缺少系统库，重新执行：

```bash
sudo npx playwright install-deps chromium
```

### 端口占用

如果默认端口冲突，改 `CC_WEB_E2E_PORT`。

### 想验证真实 Claude，而不是 fake Claude

这套 E2E 默认目标不是验证真实 Claude，而是稳定回归 Web UI、control plane、agent 的联动行为。

如果要测真实 Claude，建议单独做 smoke 流程，不要直接替换掉当前这套回归测试。
