# 公网服务器部署方案

请按你的目标选择对应目录：

## 1. 直连快速验证（无 TLS）

- 路径：`deploy-public-server/01-direct-http.md`
- 适用：测试、内网、临时验证
- 重点：`ws://` 直连、`0.0.0.0:18080`

## 2. 生产 TLS 部署（域名 / 自签名）

- 路径：`deploy-public-server/02-tls.md`
- 适用：公网长期运行
- 包含：
  - 方案 B：域名 + Let's Encrypt（推荐）
  - 方案 B'：无域名 + 自签名证书（agent 需 `-tls-skip-verify`）

## 3. 运维与升级（客户端 / 安全 / 批量 / 迁移 / 排障）

- 路径：`deploy-public-server/03-operations.md`
- 适用：上线后运维与版本演进
- 包含：
  - 客户端接入（macOS / Browser）
  - 安全加固清单
  - 多 Agent 批量部署
  - 旧版本破坏性升级（legacy token -> admin token）
  - 常见故障排查

---

说明：
- 当前推荐主路径是 `-admin-token + /admin/tokens` 生成 tenant token，再用 `/tenant/tokens` 生成 UI/Agent token。
- 若你从旧版 `-ui-token/-agent-token` 迁移，请直接看 Part 3 的升级章节。
