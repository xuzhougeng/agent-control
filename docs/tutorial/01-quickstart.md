# 01 · 快速开始（单机 cc-agent，5 分钟）

目标：在本机跑起 **cc-agent 单机版**，用 **DeepSeek API** 在终端里完成一次"用 bash 看内核版本"的对话。

> 本篇只用 cc-agent 单个二进制，不涉及 cc-control / 浏览器 UI / 远程功能。
> 想加 UI、把 agent 部署到服务器、多人共享，请看 [02-生产部署](02-deploy.md)。

> Linux / macOS / Windows 都一样。下文以 Linux/amd64 为例。

## 准备

- 一份 **DeepSeek API key**（推荐入门，便宜稳定，国内可用）
  - 申请：<https://platform.deepseek.com/api_keys>
- 想用别家（Anthropic / Qwen / Ollama / OpenAI）：见 [04-Provider 配置](04-providers.md)，步骤一致，只换两个环境变量。

## 第 1 步：下载 cc-agent 单机版

打开 [Releases 页面](https://github.com/xuzhougeng/agent-control/releases) 选最新版本，按平台下载：

| 平台 | 文件名 |
|---|---|
| Linux / amd64 | `cc-agent-linux-amd64` |
| Linux / arm64 | `cc-agent-linux-arm64` |
| macOS / Apple Silicon | `cc-agent-darwin-arm64` |
| macOS / Intel | `cc-agent-darwin-amd64` |
| Windows / amd64 | `cc-agent-windows-amd64.exe` |

Linux/macOS 一键下载最新版（按需替换平台后缀）：

```bash
mkdir -p ~/cc-agent && cd ~/cc-agent
curl -LO https://github.com/xuzhougeng/agent-control/releases/latest/download/cc-agent-linux-amd64
mv cc-agent-linux-amd64 cc-agent
chmod +x cc-agent

./cc-agent -version   # 应输出当前版本号
```

> 没装 `curl`？直接浏览器打开 Releases 页面手动下载也一样。

## 第 2 步：保存 DeepSeek API key

```bash
echo 'sk-xxxxxxxxxxxxxxxx' > ~/.cc-agent-key
chmod 600 ~/.cc-agent-key
```

把 `sk-xxxxx` 换成你在 DeepSeek 控制台拿到的 key。

## 第 3 步：启动 cc-agent REPL

```bash
mkdir -p ~/cc-agent/yard ~/cc-agent/skills.d

CC_AGENT_API_KEY="$(cat ~/.cc-agent-key)" \
CC_AGENT_BASE_URL="https://api.deepseek.com" \
./cc-agent -provider deepseek -model deepseek-chat \
           -cwd ~/cc-agent/yard \
           -skills-dir ~/cc-agent/skills.d \
           -memory ~/cc-agent/sessions.db
```

启动后看到 REPL 提示符 `you> ` 就成功了。参数解释：

- `CC_AGENT_API_KEY` ：DeepSeek API key
- `CC_AGENT_BASE_URL` ：DeepSeek 的 OpenAI 兼容入口
- `-provider deepseek -model deepseek-chat` ：用 DeepSeek 的 V3 chat 模型
- `-cwd` ：bash 工具 / `@<path>` 文件附加的工作目录（agent 只能在这里跑命令、读文件）
- `-skills-dir` ：skill JSON 加载目录,空目录也行,后面 `:reflect`/`:use` 会用到
- `-memory` ：SQLite session 历史，重启可续；不写就只在内存里

> **不带 `-control-url` 和 `-http`** 就是单机 REPL 模式，不会去连任何远端。

## 第 4 步：试一下

在 `you>` 后面输：

```
用 bash 跑 uname -r，告诉我内核版本
```

回车。终端会逐步打出：

```
▶ bash {command=uname -r}
✓ exit_code=0  6.6.87.2-microsoft-standard-WSL2

Kernel version: 6.6.87.2-microsoft-standard-WSL2
```

模型自动决定调用 `bash` 工具 → 跑 `uname -r` → 把结果总结回来。

cc-agent 默认带 8 个工具（`bash` / `read` / `write` / `grep` / `glob` / `sysinfo` / `proclist` / `logtail`），都直接可用。试试：

```
you> 当前目录有哪些文件？
you> /etc/os-release 里写了啥？
you> 这台机器 mem 用了多少？
```

## 第 5 步：感受审批闸（destructive 命令）

```
you> 用 rm -rf 删除 ~/cc-agent/yard 下所有文件
```

cc-agent 检测到 `rm -rf` 是 destructive 命令，**会在终端弹一行提示**：

```
[approval] rm -rf ~/cc-agent/yard  (reason: recursive rm)
approve? [y/N]
```

回车（或输 `n`）→ 拒绝；输 `y` → 才真跑。
被拒时模型收到 `DENIED by operator (...)` 字符串，会自然降级成更安全的方案（比如先 `ls` 再逐项删）。

destructive 列表写死在 `cc-agent/internal/tools/approval.go`，常见的 `rm` / `mkfs` / `dd of=/dev/sd*` / `shutdown` / `systemctl stop` / `git push --force` 都在内。

> 想跳过审批（脚本化场景）：加 `-full-permission`（yolo）。
> 想全拒绝（无人值守）：加 `-deny-destructive`。

## 第 6 步：把这次会话沉淀成 skill（可选）

REPL 里输：

```
you> :reflect kernel-probe 查内核版本
distilling session into skill...
✓ saved skill kernel-probe
```

skill 文件落到 `~/cc-agent/skills.d/kernel-probe.json`（启动时已经把这个目录配进去了）。下次启动会自动加载,自动路由 + `:use kernel-probe` 都能用上,对"查内核"这种任务响应更稳定。详见 [cc-agent 模块文档](../../cc-agent/README.md#skills)。

## 退出

REPL 里 `Ctrl-D` 或 `Ctrl-C`。session 已经写进 `~/cc-agent/sessions.db`，下次同样命令启动会续上。

## 下一步

- 想在浏览器 / iOS / Windows 上用 → [02-生产部署](02-deploy.md) 加 cc-control，把 agent 暴露给 UI
- 切到 Anthropic / Qwen / OpenAI / 本地 Ollama → [04-Provider 配置](04-providers.md)
- 出问题了 → [troubleshooting](troubleshooting.md)
- 想看 cc-agent 全部能力（HTTP API、自定义工具、skill 进阶）→ [`cc-agent/README.md`](../../cc-agent/README.md)
