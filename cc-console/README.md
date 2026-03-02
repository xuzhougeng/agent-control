# CC Console

用户自助控制台，部署到 `console.cc-remote.app`。

功能：邮箱验证码登录 → 从预分配池自动分配 tenant → 引导用户到 cc-web 生成 token。

## 技术栈

- Cloudflare Pages（静态前端）
- Cloudflare Pages Functions（API，即 Workers）
- Cloudflare D1（SQLite 数据库）
- Resend（发送验证码邮件）

## 部署步骤

### 1. 创建 Pages 项目

```bash
npx wrangler pages project create cc-console
```

### 2. 创建 D1 数据库

```bash
cd cc-console
npx wrangler d1 create cc-console-db
```

将输出的 `database_id` 填入 `wrangler.toml`。

### 3. 执行数据库迁移

```bash
npx wrangler d1 execute cc-console-db --remote --file=migrations/0001_init.sql
npx wrangler d1 execute cc-console-db --remote --file=migrations/0002_tenant_pool.sql
```

### 4. 配置 Secrets

```bash
npx wrangler pages secret put RESEND_API_KEY     # resend.com 获取
npx wrangler pages secret put CC_WEB_URL         # cc-web 地址，如 https://106.54.201.18
```

### 5. 预生成 Tenant 池

在能访问 cc-control 的机器上运行（脚本使用 `-sk` 跳过 TLS 验证）：

```bash
CC_CONTROL_URL=https://your-server CC_ADMIN_TOKEN=your-token \
  bash scripts/seed-tenant-pool.sh 1000
```

生成的 tenant 会批量写入 D1 的 `tenant_pool` 表，用户注册时自动分配。

### 6. 部署

```bash
npm install
npm run deploy
```

### 7. 绑定自定义域名

在 Cloudflare Pages 项目设置中添加 `console.cc-remote.app`。

## 本地开发

```bash
npm install
npx wrangler d1 execute cc-console-db --local --file=migrations/0001_init.sql
npx wrangler d1 execute cc-console-db --local --file=migrations/0002_tenant_pool.sql
npx wrangler pages dev public --d1 DB=cc-console-db
```

本地需要设置环境变量（`.dev.vars` 文件）：

```
RESEND_API_KEY=re_xxx
CC_WEB_URL=https://127.0.0.1
```

## Resend 配置

1. 注册 [resend.com](https://resend.com)（免费 100 封/天）
2. 添加域名 `cc-remote.app` 并验证 DNS
3. 创建 API Key

## 流程

```
用户输入邮箱 → API 发送 6 位验证码 → 用户输入验证码 → 验证通过
  ↓ 新用户
  从 tenant_pool 分配一个预创建的 tenant → 保存到 users 表
  ↓
  创建 session cookie → 进入 Dashboard
  ↓
  展示 Tenant Token（可复制）
  ↓
  用户点击 "Open Tenant Panel" → 跳转到 CC_WEB_URL/tenant
  ↓
  用 Tenant Token 登录，自行生成 UI Token + Agent Token
```
