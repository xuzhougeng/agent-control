# Web E2E 自动化

更偏运行步骤的说明见 [docs/how-to-test-e2e.md](/home/xzg/project/agent-control/docs/how-to-test-e2e.md)。

当前仓库的 Web 自动化测试采用：

- `Playwright` 负责浏览器断言
- 本地 `cc-control + cc-agent` harness 负责完整链路
- `tests/web-e2e/fixtures/fake-claude.py` 负责替代真实 Claude CLI
- Playwright 路由拦截 CDN 的 `xterm.js`，避免测试依赖外网

## 目录

- `tests/web-e2e/playwright.config.mjs`
- `tests/web-e2e/run-harness.sh`
- `tests/web-e2e/specs/workspace.spec.js`
- `tests/web-e2e/fixtures/fake-claude.py`

## 覆盖内容

当前首批用例覆盖：

1. Chat 视图创建统一 session 并发送消息
2. `Chat -> Terminal -> Chat` 模式切换
3. 实例列表在来回切换后保持稳定，不无限增长
4. 预先存在的外部 Claude session 能通过 PTY 接入
5. 左右抽屉可以上下拖动
6. `/chat` 兼容路径会跳转到统一 workspace 的 Chat 视图

## 运行方式

先安装依赖：

```bash
npm install
npx playwright install
```

然后运行：

```bash
npm run test:web:e2e
```

如需指定端口：

```bash
CC_WEB_E2E_PORT=18120 npm run test:web:e2e
```

## 设计说明

- Harness 使用 legacy `ui-token` / `agent-token`，避免测试里先走 token 签发流程。
- `fake-claude.py` 同时模拟：
  - `--session-id`
  - `--resume`
  - `Session ID ... is already in use`
  - `No conversation found with session ID ...`
- 对 PTY 来说，只有真正收到第一条输入后才会创建 conversation marker，用于回归“空 session 不应误 resume”的场景。
- 如果设置 `CC_WEB_E2E_CLAUDE_MODE=real`，harness 会改用系统 Claude CLI，而不是 `fake-claude.py`。这只用于单独 smoke，不建议替代默认回归。
