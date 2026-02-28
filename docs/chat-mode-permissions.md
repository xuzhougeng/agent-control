# Chat 模式权限控制指南（Claude Worker）

这份文档说明如何在 `session_type=chat` 下控制 Claude 的能力范围。

适用场景：
- 你希望 Chat 可用，但不希望它默认拥有过大权限
- 你希望按环境（Linux/Windows）或按会话动态调整权限策略

---

## 1) Chat 模式的特点

`cc-chat-claude` 使用无头 stream-json 调用 Claude CLI。  
与 PTY 模式不同，Chat 模式没有交互式审批菜单，因此权限应在会话创建前就设定好。

当前项目里，相关配置来自环境变量（`CC_CLAUDE_*`）。

---

## 2) 关键权限变量

以下变量由 `cc-agent/internal/claudecli/config.go` 读取：

- `CC_CLAUDE_CMD`
  - Claude 可执行文件路径（默认 `claude`）
- `CC_CLAUDE_PERMISSION_MODE`
  - 权限模式（默认 `dontAsk`）
- `CC_CLAUDE_ALLOWED_TOOLS`
  - 允许工具白名单
- `CC_CLAUDE_DISALLOWED_TOOLS`
  - 禁止工具黑名单
- `CC_CLAUDE_ADD_DIR`
  - 允许访问目录（逗号分隔，会映射成多个 `--add-dir`）
- `CC_CLAUDE_MODEL` / `CC_CLAUDE_EFFORT`
  - 模型和推理强度
- `CC_CLAUDE_SYSTEM_PROMPT` / `CC_CLAUDE_APPEND_SYSTEM_PROMPT`
  - 系统提示词策略
- `CC_CLAUDE_TIMEOUT_MS`
  - 超时毫秒数
- `CC_CLAUDE_PROFILE_FILE`
  - 个性化提示词文件路径（启动时读取并注入）
- `CC_CLAUDE_INJECT_RUNTIME_CONTEXT`
  - 是否注入运行时上下文（默认开启，`1`/`true`）

注意（非常重要）：
- `cc-chat-claude` 直接读取的是 `CC_CLAUDE_*` 变量。
- `CLAUDE_PATH`、`CHAT_PROFILE_FILE` 是启动脚本（如 `run-multichat.sh` / `run-multichat-win.ps1`）的输入变量；脚本会转成 `CC_CLAUDE_CMD`、`CC_CLAUDE_PROFILE_FILE` 传给 worker。
- `ALLOW_ROOT`（Linux 环境变量）和 `-AllowRoot`（Windows PowerShell 参数）是 `cc-agent` 的启动参数，不属于 `CC_CLAUDE_*`；它决定 agent 允许访问的根目录。
- 如果你是手工执行 `./bin/cc-agent -chat-worker ./bin/cc-chat-claude`，请直接设置 `CC_CLAUDE_CMD` 和 `CC_CLAUDE_PROFILE_FILE`。

---

## 3) 推荐策略（先从最小权限开始）

先用保守策略起步，再逐步放开：

1. 固定 `CC_CLAUDE_PERMISSION_MODE=dontAsk`
2. 只开放必要工具（`CC_CLAUDE_ALLOWED_TOOLS`）
3. 用 `CC_CLAUDE_ADD_DIR` 限制目录
4. 如有高风险工具，再用 `CC_CLAUDE_DISALLOWED_TOOLS` 二次兜底

示例（来自项目已有实践）：

```bash
CC_CLAUDE_ALLOWED_TOOLS="Bash(git:*) Read Edit"
```

---

## 4) Linux 使用示例

### 4.1 启动前设置（推荐）

在运行 `run-multichat.sh` 之前设置：

```bash
export CLAUDE_PATH="$(which claude-code)"
export CHAT_PROFILE_FILE="./chat-profile.md"
export ALLOW_ROOT="/workspace/repo"
export CC_CLAUDE_PERMISSION_MODE="dontAsk"
export CC_CLAUDE_ALLOWED_TOOLS="Bash(git:*) Read Edit"
export CC_CLAUDE_ADD_DIR="/workspace/repo,/workspace/repo/docs"

bash ./run-multichat.sh
```

### 4.2 说明

- 脚本会同时启动 `*-chat-claude` 和 `*-chat-echo`
- 只有 `*-chat-claude` 会使用这些 `CC_CLAUDE_*` 配置
- `CLAUDE_PATH` / `CHAT_PROFILE_FILE` 仅用于脚本入口参数；不是 worker 直接读取变量
- `ALLOW_ROOT` 会传给 `cc-agent -allow-root`
- 若未显式设置 `ALLOW_ROOT`，Linux 测试包默认使用当前 bundle 根目录作为允许根目录

### 4.3 手工启动 `cc-agent`（不走脚本）

若你直接运行：

```bash
./bin/cc-agent ... -chat-worker ./bin/cc-chat-claude
```

请使用：

```bash
export CC_CLAUDE_CMD="/home/xzg/.local/bin/claude"
export CC_CLAUDE_PROFILE_FILE="./chat-profile.md"
export CC_CLAUDE_PERMISSION_MODE="dontAsk"
export CC_CLAUDE_ALLOWED_TOOLS="Bash(git:*) Read Edit"
export CC_CLAUDE_DISALLOWED_TOOLS=""
```

---

## 5) Windows 使用示例

### 5.1 查找 Claude 路径

```powershell
Get-Command claude-code
# 或
Get-Command claude
```

### 5.2 启动前设置（推荐）

```powershell
$env:ALLOW_ROOT = "D:\repo"
$env:CC_CLAUDE_PERMISSION_MODE = "dontAsk"
$env:CC_CLAUDE_ALLOWED_TOOLS = "Bash(git:*) Read Edit"
$env:CC_CLAUDE_ADD_DIR = "D:\repo,D:\repo\docs"

powershell -ExecutionPolicy Bypass -File .\run-multichat-win.ps1 -ClaudePath "C:\path\to\claude.exe" -ChatProfileFile ".\chat-profile.md" -StartAgent 1
```

补充：

- 也可以直接传 `-AllowRoot "D:\repo"`
- 若未显式传 `-AllowRoot`，Windows 测试包默认使用当前 bundle 根目录作为允许根目录

---

## 6) 在 UI 会话级覆盖（可选）

统一 workspace 在 Chat 视图下创建会话时，可在 `env` 输入里覆盖会话级配置。  
例如：

```text
CC_CLAUDE_PERMISSION_MODE=dontAsk,CC_CLAUDE_ALLOWED_TOOLS=Bash(git:*) Read Edit
```

注意：当前 UI 的 `env` 解析是按英文逗号分隔 `KEY=VALUE`，值里尽量不要再包含逗号。

---

## 7) 启动时文件注入（推荐）

你可以把个性化内容写到一个文件里（例如 `chat-profile.md`），启动时加载：

- Linux：`CHAT_PROFILE_FILE=./chat-profile.md bash ./run-multichat.sh`
- Windows：`-ChatProfileFile .\chat-profile.md`

如果 bundle 根目录存在 `chat-profile.md`，当前脚本会自动加载它。

---

## 8) 如何判断权限是否生效

若权限过紧，`chat-claude` 回复里通常会出现类似：

- `Claude error: ... tool_not_allowed`
- 或其它 permission_denials 信息

可结合以下日志定位：

- `run/cc-agent-chat-claude.stdout.log`
- `run/cc-agent-chat-claude.stderr.log`

也可以直接看启动脚本输出和结果文件：

- 启动完成后会打印 `Allow Root: ...`
- `run/tokens-and-process.json` 里会写入 `allow_root`

这两个值对应的是 agent 实际收到的 `-allow-root` 参数，适合先确认目录边界是否符合预期。

---

## 9) 场景应用：Windows AI 助手（系统状态分析/配置）

目标：让 Chat 模式下的 Claude 作为 Windows 运维助手，能够分析系统状态，并在可控范围内执行配置命令。

### 9.1 只读分析模式（推荐先用）

适合先验证能力，不改动系统：

```powershell
$env:CC_CLAUDE_PERMISSION_MODE = "dontAsk"
$env:CC_CLAUDE_ALLOWED_TOOLS = "Read Bash(Get-*:*) Bash(ipconfig:*) Bash(systeminfo:*) Bash(wmic:*)"
$env:CC_CLAUDE_DISALLOWED_TOOLS = "Bash(Set-*:*) Bash(New-*:*) Bash(Remove-*:*) Bash(sc:*) Bash(reg:*)"
```

典型问题：
- 分析 CPU / 内存 / 磁盘占用
- 检查端口监听和进程关联
- 汇总系统版本、网络配置、服务状态

### 9.2 可配置模式（按需开启）

仅在你明确需要“让助手改配置”时开启：

```powershell
$env:CC_CLAUDE_PERMISSION_MODE = "dontAsk"
$env:CC_CLAUDE_ALLOWED_TOOLS = "Read Edit Bash(Get-*:*) Bash(Set-*:*) Bash(New-*:*) Bash(sc:*) Bash(reg:*) Bash(netsh:*)"
```

建议同时限制目录（避免误写）：

```powershell
$env:CC_CLAUDE_ADD_DIR = "D:\multichat-test-windows-amd64,D:\multichat-test-windows-amd64\run"
```

### 9.3 配合 chat-profile 注入系统提示词

可在 `chat-profile.md` 里写入固定策略，例如：

```md
# Windows Assistant Profile

你运行在 Windows 终端运维场景中。
先做信息收集，再给出变更方案；未经用户确认，不执行高风险写操作。
输出时先给结论，再列命令与影响范围。
```

启动时加载：

```powershell
powershell -ExecutionPolicy Bypass -File .\run-multichat-win.ps1 `
  -ClaudePath "C:\path\to\claude.exe" `
  -ChatProfileFile ".\chat-profile.md" `
  -AllowRoot "D:\multichat-test-windows-amd64" `
  -StartAgent 1
```

### 9.4 注意事项

- Chat 模式没有交互审批面板，权限控制要在启动前设置好。
- 涉及 `Set-*`、`reg`、`sc`、`netsh` 等命令时，建议先在“只读分析模式”完成检查，再切到“可配置模式”。
- 某些系统配置命令需要管理员权限；若权限不足，命令会失败，这是预期行为。

### 9.5 高权限模式（几乎全开放）

适合受控测试机或隔离环境，目标是让助手“几乎可以做任何事情”：

```powershell
$env:CC_CLAUDE_PERMISSION_MODE = "dontAsk"
$env:CC_CLAUDE_ALLOWED_TOOLS = "Bash(*) Read Edit"
$env:CC_CLAUDE_DISALLOWED_TOOLS = ""
```

如果你希望它可访问更大范围目录，可放宽：

```powershell
$env:CC_CLAUDE_ADD_DIR = "D:\"
```

启动示例：

```powershell
powershell -ExecutionPolicy Bypass -File .\run-multichat-win.ps1 `
  -ClaudePath "C:\path\to\claude.exe" `
  -ChatProfileFile ".\chat-profile.md" `
  -AllowRoot "D:\" `
  -StartAgent 1
```

风险提示（务必阅读）：
- 该模式下，模型可执行高风险命令（含系统配置改动、服务/网络策略修改等）。
- 仅建议在测试环境使用，不建议在生产机直接启用。
- 建议配合快照/备份；至少保留 `run/*.log` 便于审计与回滚分析。

---

## 10) Linux 高权限模式（与 Windows 对齐）

目标与 Windows 高权限模式一致：在受控测试机上尽量放开能力。

```bash
export CC_CLAUDE_PERMISSION_MODE="dontAsk"
export CC_CLAUDE_ALLOWED_TOOLS="Bash(*) Read Edit"
export CC_CLAUDE_DISALLOWED_TOOLS=""
export CC_CLAUDE_ADD_DIR="/"
```

启动示例：

```bash
export CLAUDE_PATH="$(which claude-code || which claude)"
export CHAT_PROFILE_FILE="./chat-profile.md"
export ALLOW_ROOT="/"
bash ./run-multichat.sh
```

风险提示（与 Windows 相同）：
- 该模式会显著提高命令执行范围，可能触发高风险系统改动。
- 仅建议在测试环境或隔离环境使用，不建议在生产机启用。
- 建议保留 `run/*.log`，并在执行前准备快照/备份。

---

## 11) 常见问题

`Q: 为什么 Windows 上 PTY 不可用，但 Chat 可用？`  
`A:` 当前设计就是 Windows 先走 Multi-Chat；PTY 创建会被明确拒绝。

`Q: 我放开了工具，仍然报权限问题？`  
`A:` 先确认三个点：  
1. 变量是否在启动 agent 前设置  
2. 会话 `env` 是否覆盖了全局配置  
3. Claude 可执行路径是否正确（`CC_CLAUDE_CMD` / `-ClaudePath`）

`Q: 我设置了 CLAUDE_PATH / CHAT_PROFILE_FILE，但 chat-claude 仍无响应？`  
`A:` 多数是因为你走的是“手工启动 agent”而不是启动脚本。  
手工启动时请改用 `CC_CLAUDE_CMD` / `CC_CLAUDE_PROFILE_FILE`。  

`Q: 启动脚本默认允许目录是哪个？`  
`A:` Linux 和 Windows 测试包默认都使用当前 bundle 根目录作为 `allow_root`。  
如果你显式设置了 `ALLOW_ROOT`（Linux）或 `-AllowRoot`（Windows），则以你传入的值为准。  
启动完成后可直接看控制台里的 `Allow Root: ...`，或查看 `run/tokens-and-process.json` 中的 `allow_root` 字段。  
