# 文档索引

本目录为 Agent Control 项目文档中心。按用途分类如下。写作约定见 [文档写作规范](doc-style-guide.md)。

> 关于仓库里 `cc-*` 前缀的含义（**CC = Coding Crew**）以及统一的命名词表，见 [CC = Coding Crew](coding-crew.md)。

## 版本说明

- [v0.7.3：审批超时可配置](v0.7.3-release-notes.md) — `-approval-timeout` flag / `CC_AGENT_APPROVAL_TIMEOUT` env，原 5min 写死取消
- [v0.7.2：iOS / Windows 审批卡片标记 cc-agent 来源](v0.7.2-release-notes.md) — 三端徽章一致，零协议改动
- [v0.7.1：UI 路由审批闸](v0.7.1-release-notes.md) — destructive 命令在 Web/iOS UI 上等待审批
- [v0.7.0：自研 cc-agent 上线](v0.7.0-release-notes.md) — 新模块、cc-agent 与 cc-proxy 区分、升级指南

## 入门

- [**Tutorial 教程**](tutorial/README.md) — 从本地 5 分钟跑起来到生产部署的全流程指南（v0.7.3+，**推荐新人入口**）
- [快速上手](getting-started.md) — 基于现有系统的接入测试（非本地部署）
- [使用场景](use-cases.md) — 典型使用场景与跳转

## 开发

- [架构说明](architecture.md) — 组件、协议与生命周期
- [API 参考](api.md) — REST/WS 鉴权与端点
- [聊天模式与权限](chat-mode-permissions.md) — Chat 模式权限与 run-multichat
- [多聊天 Bundle 使用](multichat-bundle-usage.md) — multichat 操作与 `CC_CLAUDE_*` 变量

## 部署

- [公网部署总览](deploy-public-server.md) — 部署模式与子文档入口
  - [直接 HTTP](deploy-public-server/01-direct-http.md)
  - [TLS（Nginx + Let's Encrypt）](deploy-public-server/02-tls.md)
  - [Cloudflare Tunnel](deploy-public-server/02a-cloudflare-tunnel.md)
  - [运维与监控](deploy-public-server/03-operations.md)
  - [后台部署指南](deploy-public-server/04-agent-background.md) — 将 cc-agent 作为后台服务（systemd/NSSM 等）

## 测试

- [如何跑 E2E 测试](how-to-test-e2e.md) — 执行步骤与环境
- [Web E2E 设计说明](web-e2e.md) — 测试范围与设计

## 合规与政策

- [隐私政策](privacy-policy.md)

---

返回 [项目根 README](../README.md)。
