# Agent Control 教程

> 适用版本：v0.7.3+

本目录是按场景写的上手教程。如果你是第一次接触本项目，按顺序读完
[01-quickstart](01-quickstart.md) 就能跑起来。

## 我应该读哪一篇？

| 你的目标 | 看这里 |
|---|---|
| 本地 5 分钟搭一套体验一下 | [01-快速开始](01-quickstart.md) |
| 把 cc-agent 部署到一台 Linux 服务器做运维 | [02-生产部署](02-deploy.md) |
| 学会浏览器 / iOS / Windows UI 怎么用 | [03-使用指南](03-using-ui.md) |
| 切换或配置 LLM provider（DeepSeek / Anthropic / Ollama / Qwen） | [04-Provider 配置](04-providers.md) |
| 报错了，排查 | [troubleshooting](troubleshooting.md) |

## 项目术语

- **cc-control** — 控制平面，HTTP + WebSocket 服务，统一管 token / session / 审计。**只跑一份，所有 server 共享。**
- **cc-agent** — *自研* server-ops agent。自己跑 LLM 主循环 + 调工具（bash/read/grep/sysinfo/...）。每台服务器跑一个。**v0.7.0 起的新模块。**
- **cc-proxy** — *旧* PTY 代理。包外部 Claude Code / Codex / Gemini CLI。**v0.7.0 之前叫 cc-agent，改名了。**
- **cc-web** — 静态浏览器 UI。由 cc-control 内置提供。
- **AgentControlMac / AgentControlWin** — iOS / macOS / Windows 原生客户端。

cc-agent 和 cc-proxy 都通过同一套 WS 协议接入 cc-control，UI 上用紫色 `cc-agent` 徽章区分。

## 架构一图流

```
                        ┌─────────────────────┐
                        │  cc-control (HTTP+WS) │
                        │  · token / session    │
                        │  · audit / approval   │
                        └─┬─────────────────┬───┘
        WS chat / WS PTY  │                 │  HTTP / WS
                          │                 │
        ┌─────────────────┴──────┐    ┌─────┴────────────────┐
        │  cc-agent (server X)   │    │  cc-web / iOS / Win  │
        │  · LLM 主循环          │    │  · UI 操作面          │
        │  · 8 个内置工具         │    │  · 审批 / Pending    │
        │  · destructive 审批     │    │  · skill 列表         │
        └────────────────────────┘    └──────────────────────┘
                          │
        ┌─────────────────┴──────┐
        │  cc-proxy (server Y)   │
        │  · 包 Claude Code / Codex CLI │
        └────────────────────────┘
```

一个团队通常跑 1 个 cc-control + N 个 server 节点（每个节点是 cc-agent 或 cc-proxy 任选）。

## 还想读什么

- [架构说明](../architecture.md) — 协议、生命周期、内部模型详解
- [API 参考](../api.md) — REST/WS 端点
- [公网部署总览](../deploy-public-server.md) — TLS / Cloudflare Tunnel / 运维
- [v0.7.x 版本说明](../README.md#版本说明) — 每个版本带来什么
- [`cc-agent/README.md`](../../cc-agent/README.md) — cc-agent 模块文档（含 Roadmap）
