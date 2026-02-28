# Multi-Chat 测试包使用说明

这份文档用于快速回忆两件事：

1. 如何在任意支持 Bash 的环境里打包测试包
2. 如何在 Linux / Windows 目标机上直接运行测试包

Chat 权限控制专项文档见：

- `docs/chat-mode-permissions.md`

---

## 1) 一键打包（Bash）

在仓库根目录执行：

```bash
bash scripts/build-multichat-test-bundle.sh --targets linux,windows --arch amd64
```

默认输出到 `dist/`：

- `dist/multichat-test-linux-amd64/`
- `dist/multichat-test-linux-amd64.tar.gz`
- `dist/multichat-test-windows-amd64/`
- `dist/multichat-test-windows-amd64.tar.gz`
- `dist/multichat-test-windows-amd64.zip`（仅当本机有 `zip` 命令时生成）

只打 Linux：

```bash
bash scripts/build-multichat-test-bundle.sh --targets linux
```

只打 Windows：

```bash
bash scripts/build-multichat-test-bundle.sh --targets windows
```

自定义输出目录：

```bash
bash scripts/build-multichat-test-bundle.sh --out-base /path/to/output
```

---

## 2) Linux 目标机运行

解压 Linux 包后进入目录，执行：

```bash
bash ./run-multichat.sh
```

停止：

```bash
bash ./stop-multichat.sh
```

该命令会启动：

1. 一个 `cc-control`
2. 三个 `cc-agent`：
   - `*-pty`（PTY 模式）
   - `*-chat-claude`（Chat + `cc-chat-claude`）
   - `*-chat-echo`（Chat + `cc-chat-echo`）

可选环境变量（示例）：

```bash
CONTROL_PORT=18080 SERVER_ID_PREFIX=srv-linux-01 START_AGENT=1 bash ./run-multichat.sh
```

指定个性化提示词文件：

```bash
CHAT_PROFILE_FILE=./chat-profile.md bash ./run-multichat.sh
```

Claude 路径自动探测顺序：`CLAUDE_PATH` -> `claude-code` -> `claude`。
如果探测不到，请显式指定：

```bash
CLAUDE_PATH=/full/path/to/claude-code bash ./run-multichat.sh
```

运行完成后会输出：

- `tenant token`
- `ui token`
- `agent token`
- `Chat URL`
- `Allow Root`

并写入结果文件：

- `./run/tokens-and-process.json`

关于统一 `session_id` 的补充：

- Web UI 中同一个 `session_id` 会在 Chat / PTY 间复用。
- 若目标机本机已存在 `~/.claude/session-env/<session_id>`，PTY 会自动尝试用 `claude --resume <session_id>` 接入已有 Claude conversation。
- 若只是新建了逻辑 session，但 Claude conversation 实际还没建立（例如 PTY/Chat 都没真正对 Claude 说过话），切回 PTY 时系统会继续使用 `--session-id`，避免触发 `no conversation found with session ID ...`。

---

## 3) Windows 目标机运行

解压 Windows 包后进入目录，在 PowerShell 中执行：

```powershell
powershell -ExecutionPolicy Bypass -File .\run-multichat-win.ps1
```

停止：

```powershell
powershell -ExecutionPolicy Bypass -File .\stop-multichat-win.ps1
```

该命令会启动：

1. 一个 `cc-control`
2. 三个 `cc-agent`：
   - `*-pty`（PTY 模式）
   - `*-chat-claude`（Chat + `cc-chat-claude`）
   - `*-chat-echo`（Chat + `cc-chat-echo`）

可选参数（示例）：

```powershell
powershell -ExecutionPolicy Bypass -File .\run-multichat-win.ps1 -ControlPort 18080 -ServerIDPrefix srv-win-01 -ChatProfileFile .\chat-profile.md -StartAgent 1
```

说明：如果你是从 `cmd.exe` 调用 `powershell -File`，不要使用 `-StartAgent:$true` 这种写法，建议用 `-StartAgent 1` 或 `-StartAgent true`。

Claude 路径自动探测顺序：`-ClaudePath` -> `claude-code` -> `claude`。
如果探测不到，请显式指定：

```powershell
powershell -ExecutionPolicy Bypass -File .\run-multichat-win.ps1 -ClaudePath "C:\path\to\claude.exe"
```

运行完成后同样会输出 token，并写入：

- `.\run\tokens-and-process.json`

说明：

- Windows 当前仍以 Chat 测试为主；`session_type=pty` 依旧不支持。
- 统一 `session_id`、`active_instance_id` 和实例历史的语义与 Linux 端保持一致。

---

## 4) 常见问题

`Q: 为什么要保留 Windows 的 .ps1？`

`A:` 打包步骤统一用 Bash；但 Windows 目标机实际启动服务时，使用 PowerShell 脚本最稳妥，因此包内保留 `run-multichat-win.ps1`。

`Q: 这个流程是否包含 GPT 模式？`

`A:` 当前是 Multi-Chat 优先流程，默认使用 `cc-chat-echo` 作为 chat worker，不包含 GPT 模式。

`Q: 运行时提示 unauthorized（创建 tenant token 失败）怎么办？`

`A:` 常见原因是端口上已有其他服务，或者 `-AdminToken` 与实际控制面不一致。建议：

1. 换端口重试：`-ControlPort 18081`
2. 显式指定管理员 token：`-AdminToken <your-admin-token>`

补充：本项目 `cc-control` 默认会预置 legacy `ui-token`。当前脚本已显式关闭 legacy token 预置并只使用 `-admin-token`，请使用最新版脚本/最新打包产物。

`Q: Windows 上 PTY 测试会怎样？`

`A:` 预期是创建 PTY 会话时报不支持（这是当前设计），但 chat-claude / chat-echo 会正常可测。

`Q: 为什么我传入一个已经在服务器上通过命令行 claude 启动过的 session_id，第一次进 PTY 仍然会报冲突？`

`A:` 请确认目标机本机存在 `~/.claude/session-env/<session_id>`。最新版 agent 会在发现该文件时自动把 PTY 启动参数从 `--session-id` 切换为 `--resume`。如果仍报错，优先检查最新 bundle 是否已重新打包，以及 `run/cc-agent-pty.stdout.log` / `run/cc-agent-pty.stderr.log`。

`Q: 为什么新建了一个带 session_id 的空会话，PTY -> Chat -> PTY 后会看到 no conversation found？`

`A:` 旧版本会把“存在历史实例”误判成“Claude conversation 已存在”，导致错误地对 PTY 使用 `--resume`。最新版已修正：只有会话已产生真实 chat history 时，控制面才会主动要求 PTY `resume`；否则仍使用 `--session-id`。如果还有问题，请确认使用的是最新重新打包的测试包。 
