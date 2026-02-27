# Multi-Chat 测试包使用说明

这份文档用于快速回忆两件事：

1. 如何在任意支持 Bash 的环境里打包测试包
2. 如何在 Linux / Windows 目标机上直接运行测试包

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

并写入结果文件：

- `./run/tokens-and-process.json`

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
powershell -ExecutionPolicy Bypass -File .\run-multichat-win.ps1 -ControlPort 18080 -ServerIDPrefix srv-win-01 -StartAgent 1
```

说明：如果你是从 `cmd.exe` 调用 `powershell -File`，不要使用 `-StartAgent:$true` 这种写法，建议用 `-StartAgent 1` 或 `-StartAgent true`。

Claude 路径自动探测顺序：`-ClaudePath` -> `claude-code` -> `claude`。
如果探测不到，请显式指定：

```powershell
powershell -ExecutionPolicy Bypass -File .\run-multichat-win.ps1 -ClaudePath "C:\path\to\claude.exe"
```

运行完成后同样会输出 token，并写入：

- `.\run\tokens-and-process.json`

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
