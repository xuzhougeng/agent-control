# 使用场景

以下为典型使用场景及对应文档入口。

## 本地开发与联调

在本机同时跑 cc-control、cc-agent 和 Web UI，快速验证功能或调试。

- **步骤**：[快速上手](getting-started.md)（依赖 [README Quick Start](../README.md#quick-start)）
- **Token**：用 Admin 创建 Tenant，再在 Tenant 页生成 UI + Agent Token
- **Chat 模式**：README 中 [Chat Mode Quick Start](../README.md#chat-mode-quick-start) 配置 `-chat-worker`（如 cc-chat-echo / cc-chat-claude）

## 多租户 / 团队共用控制面

一个 cc-control 服务多团队，每个租户独立 Token、独立 Agent 与会话。

- **模型说明**：[架构 - 认证与租户隔离](architecture.md#3-认证与租户隔离)
- **Token 流程**：[API - Token 与鉴权](api.md) 及 [README - Token Model](../README.md#token-model-latest)
- **管理入口**：`/admin` 创建 Tenant Token，`/tenant` 由租户自助签发 UI/Agent Token

## PTY 终端 + Chat 统一会话

在同一会话内切换终端（PTY）与聊天（Chat），共用同一 `session_id`，支持 Claude 会话恢复。

- **产品行为**：README [Unified Session ID](../README.md#unified-session-id)、[Chat Mode Quick Start](../README.md#chat-mode-quick-start)
- **权限与多聊天**：[聊天模式与权限](chat-mode-permissions.md)、[多聊天 Bundle 使用](multichat-bundle-usage.md)

## 公网或生产部署

将 control 暴露到公网、配置 TLS、多台 Agent 或 Cloudflare Tunnel 等。

- **入口**：[公网部署总览](deploy-public-server.md)
- **Agent 常驻**：[后台部署指南](deploy-public-server/04-agent-background.md)（Linux systemd / Windows NSSM）

## 原生客户端（macOS / iOS）

使用 AgentControl 原生 App 接入同一控制面，协议与 Web 一致。

- **部署与运维**：[运维与升级](deploy-public-server/03-operations.md) 中客户端接入说明
- **项目位置**：`app/AgentControlMac/`
