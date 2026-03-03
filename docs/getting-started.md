# 快速上手

本文档是**基于现有系统的接入测试**，不是本地环境部署测试。  
你不需要在本机启动 `cc-control`，而是直接连接到已部署好的中心服务完成验证。

## 测试流程（基于现有系统）

1. **获取凭证（Console 页面）**  
   在 Console 页面先记录 `TENANT_ID` 和 `TENANT_TOKEN`。

2. **进入 Tenant Panel 创建 Token**  
   点击 `Open Tenant Panel`，使用 `TENANT_TOKEN` 登录后创建：
   - `UI Token`：用于网页端登录与页面操作。
   - `Agent Token`：用于 `cc-agent` 连接中心服务。  
   注意：`UI Token` 与 `Agent Token` 必须区分，不能混用。

3. **安装并启动 Agent（Linux）**  
   使用发布包安装并启动 `cc-agent`（可选配 `cc-chat-claude`）：
   - 必填参数：`-control-url`、`-agent-token`、`-server-id`
   - 常用可选参数：`-allow-root`、`-chat-worker`、`-claude-path`
   - `-tls-skip-verify` 仅用于自签名或证书不完整场景
   - `-agent-token` 必须使用 **Agent Token**

4. **网页端连接验证**  
   使用 `UI Token` 登录 Web 工作区后：
   - 进入左侧 `Sessions`
   - 打开 `Workspace Task`
   - 创建一个 Session 并开始测试  
   若未创建 Session，页面不显示会话内容属于正常行为。

## Windows 说明

- Windows 端目前仅支持 Chat 模式。
- Windows 端不支持 PTY（终端模拟），无法提供交互式 shell 会话。
- 需要终端类操作时，请使用 Linux/macOS Agent。

## 下一步

- **公网或生产部署**：见 [公网部署总览](deploy-public-server.md)（直连 / TLS / Cloudflare Tunnel、运维）。
- **将 cc-agent 作为后台服务长期运行**：见 [后台部署指南](deploy-public-server/04-agent-background.md)（systemd、NSSM、nohup 等）。
- **典型使用场景**：见 [使用场景](use-cases.md)。
