# iOS 应用上架美区 Apple App Store 指南

适用于 **CC Remote** iOS 应用，从零到提交审核的完整流程。

---

## 一、前置条件

| 项目 | 说明 |
|------|------|
| **Apple Developer Program** | $99/年，[developer.apple.com](https://developer.apple.com) 注册 |
| **Xcode** | 15+，从 Mac App Store 安装 |
| **项目** | `app/AgentControlMac`，选 **AgentControliOS** scheme |

---

## 二、Apple Developer 与 App Store Connect

### 2.1 开发者账号

登录 [Apple Developer](https://developer.apple.com/account) → **Membership** 确认状态为 **Active**。

### 2.2 在 App Store Connect 创建应用

1. 打开 [App Store Connect](https://appstoreconnect.apple.com) → **我的 App** → **+** → **新建 App**。
2. 填写：
   - **平台**：iOS
   - **名称**：CC Remote
   - **主要语言**：英语（美国）
   - **套装 ID**：选与 Xcode 一致的 Bundle ID（如 `com.agentcontrol.ios`，需先在 Certificates, Identifiers & Profiles 中创建）
   - **SKU**：自定义唯一字符串，如 `cc-remote-ios-001`
   - **用户访问**：完整访问（若无需登录即用选此项）

### 2.3 Bundle ID 与描述文件

1. [developer.apple.com/account → Identifiers](https://developer.apple.com/account/resources/identifiers/list) → **+** 添加 **App IDs**。
2. 选 **App**，Description 填 `CC Remote iOS`，Bundle ID 选 **显式**，如：`com.yourcompany.agentcontrol.ios`（与 `project.yml` 中 `PRODUCT_BUNDLE_IDENTIFIER` 一致）。
3. 若用 Xcode 自动签名：在 Xcode 中勾选 **Automatically manage signing**，Team 选你的开发者账号，Xcode 会生成/使用描述文件。
4. 若手动：在 [Profiles](https://developer.apple.com/account/resources/profiles/list) 新建 **App Store** 类型 Distribution Profile，选上述 App ID 和对应 Distribution 证书。

---

## 三、Xcode 工程配置

### 3.1 Bundle ID 与版本

在 `project.yml` 中，iOS target 已包含：

- `PRODUCT_BUNDLE_IDENTIFIER: com.agentcontrol.ios`（可改为你的，如 `com.yourcompany.agentcontrol.ios`）
- `MARKETING_VERSION: "1.0.0"`（用户可见版本）
- `CURRENT_PROJECT_VERSION: "1"`（构建号，每次上传需递增）

修改后执行：

```bash
cd app/AgentControlMac
xcodegen
```

### 3.2 签名与 Capabilities

1. 在 Xcode 打开 `AgentControl.xcodeproj`，选中 **AgentControliOS** target。
2. **Signing & Capabilities**：
   - 勾选 **Automatically manage signing**
   - **Team** 选你的 Apple Developer 团队
   - **Bundle Identifier** 与 App Store Connect / Identifiers 中一致
3. 若无推送、iCloud 等，无需额外 Capability。

### 3.3 图标与启动图

- 图标：`Resources/Assets.xcassets/AppIcon.appiconset` 已包含多尺寸，确保含 1024×1024（App Store 必需）。
- 启动屏：`Info.plist` 中 `UILaunchScreen` 为空时使用系统默认；如需自定义可在 Asset Catalog 增加 Launch Image 或 Launch Screen storyboard。

### 3.4 隐私与 ATS（重要）

当前 `Resources/iOS/Info.plist` 含 `NSAllowsArbitraryLoads: true`，便于连接自建 HTTP/HTTPS 服务器。**App Review 可能要求说明理由或收紧配置**。

- **建议**：在 App Store Connect 的 **App 隐私** 与 **审核备注** 中说明：应用用于连接用户自行部署的 cc-control 服务端，需允许用户配置的任意 Base URL（含 HTTP 及自签名 HTTPS）。
- 若审核要求收紧：可改为只允许用户输入的 host 加入 `NSExceptionDomains`，或仅允许 HTTPS + 自签名例外，避免全局 `NSAllowsArbitraryLoads`。

---

## 四、App Store  listing 必填项

在 App Store Connect 对应 App → **App 信息** / **定价与销售** / **App 隐私** 等页面完成：

| 项 | 说明 |
|----|------|
| **副标题** | 简短一句，如 "Terminal client for cc-control server" |
| **描述** | 英文说明功能、适用场景（连接 cc-control 服务、会话管理、终端等） |
| **关键词** | 英文逗号分隔，如 "terminal,agent,control,claude" |
| **支持 URL** | 可填项目 GitHub 或文档链接 |
| **营销 URL** | 可选 |
| **推广文本 (Promotional Text)** | 可选，170 字符内；展示在描述上方，可随时改无需发版 |
| **隐私政策 URL** | **必填**（提交审核前）。可用仓库内 `docs/privacy-policy.md` 转为页面或 GitHub Pages 链接 |
| **类别** | 如 **开发者工具** 或 **工具** |
| **分级** | 按问卷填写，通常 4+ |
| **定价** | 免费或付费；若付费需配置税务与银行信息 |
| **销售范围** | 勾选 **美国** 及其他需要上架的国家/地区 |
| **版权/权利所有人** | 格式：`年份 权利人姓名或实体`，如 `2025 Your Name`；勿填 URL |

### 英文（美国）必填项 — 直接复制粘贴

解决「你在此页上有一个或多个错误」时，在 **App Store Connect → 对应 App → App 信息** 中为 **英语（美国）** 填写以下三项：

**1. 技术支持网址 (Technical Support URL)**（必填）

- 若项目已放 GitHub，用仓库地址，例如：`https://github.com/你的用户名/agent-control`
- 若用文档站，可用：`https://你的域名/docs` 或仓库的 GitHub Pages（如 `https://用户名.github.io/agent-control/`）
- 临时可先填：`https://github.com`（后续在 App 信息中改为实际支持页）

**2. 关键词 (Keywords)**（必填，100 字符以内，逗号分隔无空格）

```
terminal,agent,control,claude,remote,server,session,developer
```

**3. 描述 (Description)**（必填，4000 字符以内）

```
CC Remote connects to your cc-control server so you can manage AI coding sessions (Claude Code, Codex, OpenCode, etc.) from your iPhone or iPad.

• View servers and sessions — See which servers are online and list sessions per server.
• Full terminal — Create or resume sessions with a real terminal (SwiftTerm); input, output, and resize work as expected.
• Approval queue — When the agent needs confirmation (e.g. approve/reject prompts), handle them with one tap.
• Your data stays yours — You configure the server URL and token; the app talks only to your own cc-control instance.

Requires a running cc-control server (see project docs for deployment). Supports HTTP, HTTPS, and self-signed certificates (optional TLS verification skip in Settings).
```

**4. 推广文本 (Promotional Text)**（可选，170 字符内）

```
Manage Claude Code and other AI agent sessions from your iPhone. Connect to your cc-control server, run the terminal, and approve prompts on the go.
```

**5. 隐私政策网址 (Privacy Policy URL)**（必填，才能开始审核）

- 仓库内已提供 `docs/privacy-policy.md`，需以 **可公网访问的 URL** 形式提供。
- **方式 A — GitHub Pages**：把 `docs/` 设为 Pages 源，则地址为：  
  `https://你的用户名.github.io/agent-control/privacy-policy`  
  （若仓库名为 `agent-control`；若 Pages 根目录是项目根，路径可能为 `/docs/privacy-policy`，以实际为准。）
- **方式 B — 自有域名**：把 `privacy-policy.md` 转成 HTML 放到站点，例如：`https://你的域名/privacy-policy`。
- **方式 C — 用 GitHub 原始文件**（不推荐，排版差）：  
  `https://github.com/你的用户名/agent-control/blob/main/docs/privacy-policy.md`  
  或 Raw：`https://raw.githubusercontent.com/你的用户名/agent-control/main/docs/privacy-policy.md`（多数浏览器会直接显示 Markdown 文本，可读但简陋）。

在 App Store Connect → **App 信息** → **英语（美国）** → **隐私政策网址** 中填入上述任一有效 URL 即可。

**6. 版权/权利所有人 (Copyright / Rights Owner)**（必填，勿填 URL）

- 格式：**年份 + 空格 + 拥有该 App 专有权的人员或实体名称**。
- 年份：通常填首次发布或获得权利的年份（如 2025）。
- 名称：个人填本人姓名（英文），公司填公司名（如 Acme Inc.）。

示例（请改成你自己的年份和名称）：

```
2025 Your Name
```

或公司：

```
2025 Your Company Inc.
```

### 截图（必传尺寸）

App Store Connect **强制要求** 至少提供以下两种尺寸的截屏，否则无法提交：

| 设备 | 尺寸要求 | 像素（竖屏 × 横屏） |
|------|----------|---------------------|
| **6.5 英寸 iPhone** | 必传 | 1242×2688、2688×1242、1284×2778 或 2778×1284 |
| **13 英寸 iPad** | 必传 | 2064×2752、2752×2064、2048×2732 或 2732×2048 |

每种尺寸至少 3 张、最多 10 张；可多张展示不同界面（服务器列表、终端、审批队列等）。

**用模拟器生成截屏：**

1. 在 Xcode 中选 **AgentControliOS** scheme，顶部设备选：
   - **iPhone 15 Plus** 或 **iPhone 14 Plus**（对应 6.5"）
   - **iPad Pro 12.9-inch (6th generation)** 或 **iPad Pro 13-inch (M4)**（对应 13"）。
2. **Product** → **Run** 在模拟器里启动 App，进入要截的界面。
3. 模拟器菜单 **File** → **Save Screen**（或 **⌘S**），截图会保存到桌面，分辨率即上述尺寸。
4. 分别用 6.5" iPhone 模拟器和 13" iPad 模拟器各截至少 3 张，在 App Store Connect 对应 App 版本页的 **截屏** 里，上传到「6.5 英寸 iPhone 显示屏」和「13 英寸 iPad 显示屏」槽位。

若无 13" iPad 模拟器：Xcode → **Window** → **Devices and Simulators** → **Simulators** → 左下 **+** 添加 **iPad Pro 12.9-inch** 或 **iPad Pro 13-inch (M4)**。

---

## 五、归档与上传

### 5.1 真机与版本

- 在 Xcode 顶部选 **Any iOS Device (arm64)**（勿选模拟器）。
- 确认 **Product → Scheme → Edit Scheme → Run** 的 Build Configuration 为 **Release**（Archive 默认用 Release）。

### 5.2 Archive

1. **Product** → **Archive**。
2. 若失败：检查签名（Team、Bundle ID）、证书是否有效、`CURRENT_PROJECT_VERSION` 是否已递增。
3. 成功后弹出 **Organizer**，选中刚生成的 Archive。

### 5.3 上传到 App Store Connect

1. 在 Organizer 中点击 **Distribute App**。
2. 选 **App Store Connect** → **Upload**。
3. 选项：勾选 **Upload your app's symbols**（便于崩溃分析），**Manage Version and Build Number** 按需。
4. 选择签名：**Automatically manage signing** 或你已有的 Distribution 描述文件。
5. 等待上传完成。

---

## 五.5 TestFlight 测试方案

在正式提交 App 审核前，建议先用 **TestFlight** 做内测/外测，验证安装、连接服务端、终端与审批流程是否正常。

### 前置条件

- 已完成 **五、归档与上传**，且该构建在 App Store Connect 中显示为「处理中」或「可供测试」。
- 测试者使用 **Apple ID 邮箱**（无需开发者账号）。

### 步骤一：在 App Store Connect 启用 TestFlight

1. 打开 [App Store Connect](https://appstoreconnect.apple.com) → 选择 **CC Remote** → 左侧 **TestFlight**。
2. 若首次使用：在 **App 信息** 中补全 **出口合规**、**内容版权**、**广告标识符** 等问卷（与正式上架相同），否则无法启用测试。
3. 上传的构建处理完成后会出现在 **iOS 构建版本** 中；首次需等 **Beta 版 App 审核**（通常几小时到 1 天），通过后状态为「可供测试」。

### 步骤二：添加测试员

**内部测试（最多 100 人）**

- **TestFlight** → **内部测试** → **+** 创建群组（如 "Core Team"）。
- 添加成员：输入 Apple ID 对应邮箱，成员须在 [App Store Connect → 用户与访问 → 用户](https://appstoreconnect.apple.com/access/users) 中且角色为 **Admin**、**App 管理员** 或 **开发者**。
- 在群组中勾选刚通过的构建，保存；成员会收到邮件，按邮件指引在 iPhone 上安装 **TestFlight** App 并安装 CC Remote。

**外部测试（最多 10,000 人）**

- **TestFlight** → **外部测试** → **+** 创建群组（如 "Beta Testers"）。
- 勾选构建版本，填写 **测试信息**（给测试者看的说明、测试重点、服务器连接示例等）。
- **提交 Beta 版 App 审核**（仅外部测试需要）；通过后添加**外部测试员**：输入邮箱或分享公开链接，测试者通过链接或邮件接受邀请即可安装。

### 步骤三：测试要点（CC Remote 建议清单）

| 项目 | 说明 |
|------|------|
| 安装与启动 | 从 TestFlight 安装后能正常打开、无闪退 |
| 设置 | 在设置页配置 Base URL、UI Token，保存后生效 |
| 连接 | 连接你部署的 cc-control（HTTP/HTTPS/自签名），列表能拉取服务器与会话 |
| 终端 | 创建/恢复会话，终端输入输出、快捷键栏（Esc/Tab/Ctrl+C 等）正常 |
| 审批 | 有待审批时出现队列，Approve/Reject 可点击且服务端有响应 |
| 前后台 | 切到后台再回来，WebSocket 重连、状态指示正常 |

### 步骤四：收集反馈与迭代

- 测试者在 TestFlight 内可 **发送反馈**（带截图），或在 **崩溃** 时选择提交诊断信息。
- 在 App Store Connect → **TestFlight** → **崩溃与反馈** 中查看。
- 修 bug 后递增 `CURRENT_PROJECT_VERSION`，重新 Archive 并上传；新构建出现在 TestFlight，可替换当前测试构建或新建测试群组。

### 与正式上架的关系

- TestFlight 使用的构建与正式上架**可以是同一个**：先只勾选在 TestFlight 群组中供测试，确认无误后，在 **App Store** 页签下该版本 **构建版本** 处选同一构建，再点「提交以供审核」即可。
- 也可在测试通过后上传新构建仅用于正式版；版本号/构建号按需递增。

---

### 5.4 选择构建版本并提交审核

1. 在 [App Store Connect](https://appstoreconnect.apple.com) 对应 App → **iOS App** → **+ 版本** 或选已有版本。
2. **构建版本** 旁点 **+**，选刚上传的构建（需处理完成后才出现）。
3. 填写 **此版本的新增内容**（What’s New）。
4. 检查 **出口合规**、**内容版权**、**广告标识符** 等问卷。
5. 在 **App 审核** 栏填 **审核备注**（可选）：说明 NSAllowsArbitraryLoads 用途、测试账号/服务器等，便于过审。
6. 点击 **提交以供审核**。

---

## 六、提交后与常见问题

- **状态**：在 App Store Connect 查看 **App 审核** 状态（等待审核、审核中、被拒、已通过）。
- **被拒**：查邮件与 Resolution Center，按条款修改 app 或 listing 后重新上传构建或更新说明再提交。
- **NSAllowsArbitraryLoads**：若因网络安全被拒，在备注中说明“用户配置的自托管服务器”，或按上文收紧 ATS 并说明例外仅用于用户指定域名。

---

## 七、本仓库相关路径速查

| 内容 | 路径 |
|------|------|
| iOS 工程配置 | `app/AgentControlMac/project.yml`（AgentControliOS target） |
| iOS Info.plist | `app/AgentControlMac/Resources/iOS/Info.plist` |
| 图标资源 | `app/AgentControlMac/Resources/Assets.xcassets/AppIcon.appiconset/` |
| 生成 Xcode 工程 | `cd app/AgentControlMac && xcodegen` |

---
