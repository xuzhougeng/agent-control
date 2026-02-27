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
export CC_CLAUDE_PERMISSION_MODE="dontAsk"
export CC_CLAUDE_ALLOWED_TOOLS="Bash(git:*) Read Edit"
export CC_CLAUDE_ADD_DIR="/workspace/repo,/workspace/repo/docs"

bash ./run-multichat.sh
```

### 4.2 说明

- 脚本会同时启动 `*-chat-claude` 和 `*-chat-echo`
- 只有 `*-chat-claude` 会使用这些 `CC_CLAUDE_*` 配置

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
$env:CC_CLAUDE_PERMISSION_MODE = "dontAsk"
$env:CC_CLAUDE_ALLOWED_TOOLS = "Bash(git:*) Read Edit"
$env:CC_CLAUDE_ADD_DIR = "D:\repo,D:\repo\docs"

powershell -ExecutionPolicy Bypass -File .\run-multichat-win.ps1 -ClaudePath "C:\path\to\claude.exe" -StartAgent 1
```

---

## 6) 在 UI 会话级覆盖（可选）

`/chat` 页面创建会话时，可在 `env` 输入里覆盖会话级配置。  
例如：

```text
CC_CLAUDE_PERMISSION_MODE=dontAsk,CC_CLAUDE_ALLOWED_TOOLS=Bash(git:*) Read Edit
```

注意：当前 UI 的 `env` 解析是按英文逗号分隔 `KEY=VALUE`，值里尽量不要再包含逗号。

---

## 7) 如何判断权限是否生效

若权限过紧，`chat-claude` 回复里通常会出现类似：

- `Claude error: ... tool_not_allowed`
- 或其它 permission_denials 信息

可结合以下日志定位：

- `run/cc-agent-chat-claude.stdout.log`
- `run/cc-agent-chat-claude.stderr.log`

---

## 8) 常见问题

`Q: 为什么 Windows 上 PTY 不可用，但 Chat 可用？`  
`A:` 当前设计就是 Windows 先走 Multi-Chat；PTY 创建会被明确拒绝。

`Q: 我放开了工具，仍然报权限问题？`  
`A:` 先确认三个点：  
1. 变量是否在启动 agent 前设置  
2. 会话 `env` 是否覆盖了全局配置  
3. Claude 可执行路径是否正确（`CC_CLAUDE_CMD` / `-ClaudePath`）
