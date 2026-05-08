# 01 · 快速开始（本地 5 分钟）

目标：在自己电脑上跑起 cc-control + cc-agent + 浏览器 UI，用 DeepSeek 完成一次"用 bash 看内核版本"的对话。

> Linux / macOS 一致。Windows 自己装请看 [02-生产部署](02-deploy.md) 里的 Windows 段落。

## 准备

- Go ≥ 1.25 或 直接下载 [v0.7.3 二进制](https://github.com/xuzhougeng/agent-control/releases/tag/v0.7.3)
- 一份 LLM API key（推荐 DeepSeek，便宜稳定）
  - 申请：<https://platform.deepseek.com/api_keys>

## 第 1 步：下载二进制

```bash
mkdir -p ~/cc-stack && cd ~/cc-stack
curl -L -o cc-control https://github.com/xuzhougeng/agent-control/releases/download/v0.7.3/cc-control-linux-amd64
curl -L -o cc-agent https://github.com/xuzhougeng/agent-control/releases/download/v0.7.3/cc-agent-linux-amd64
chmod +x cc-control cc-agent
```

> **macOS**：把 `linux-amd64` 换成 `darwin-arm64`（M 系列）或 `darwin-amd64`（Intel）。
> **Windows**：换成 `windows-amd64.exe`。

## 第 2 步：保存 LLM key

```bash
echo 'sk-xxxxxxxxxxxxxxxx' > ~/.cc-agent-key
chmod 600 ~/.cc-agent-key
```

## 第 3 步：起 cc-control

```bash
mkdir -p ~/cc-stack/web
cd ~/cc-stack
# 把 cc-web 静态资源克隆下来（约 200 KB）
curl -sL https://github.com/xuzhougeng/agent-control/archive/v0.7.3.tar.gz \
  | tar xz -C web --strip-components=1 agent-control-v0.7.3/cc-web

./cc-control \
  -addr :18180 \
  -ui-dir web/cc-web \
  -admin-token admin-dev-token \
  -ui-token "" \
  -agent-token "" \
  -audit-path audit.jsonl \
  -offline-after-sec 30 &
```

## 第 4 步：签 token

```bash
# tenant token
TENANT=$(curl -s -X POST http://127.0.0.1:18180/admin/tokens \
  -H "Authorization: Bearer admin-dev-token" \
  -H "Content-Type: application/json" -d '{"type":"tenant"}' \
  | python3 -c "import json,sys;print(json.load(sys.stdin)['token'])")

# UI + agent token
RESP=$(curl -s -X POST http://127.0.0.1:18180/tenant/tokens \
  -H "Authorization: Bearer $TENANT" \
  -H "Content-Type: application/json" -d '{"role":"owner"}')

UI_TOKEN=$(echo "$RESP" | python3 -c "import json,sys;print(json.load(sys.stdin)['ui']['token'])")
AGENT_TOKEN=$(echo "$RESP" | python3 -c "import json,sys;print(json.load(sys.stdin)['agent']['token'])")
echo "UI_TOKEN=$UI_TOKEN"
echo "AGENT_TOKEN=$AGENT_TOKEN"
```

记一下 `UI_TOKEN`（一会儿浏览器登录用）和 `AGENT_TOKEN`（cc-agent 启动用）。

## 第 5 步：起 cc-agent

```bash
mkdir -p ~/cc-stack/yard
CC_AGENT_API_KEY="$(cat ~/.cc-agent-key)" \
CC_AGENT_BASE_URL="https://api.deepseek.com" \
./cc-agent \
  -provider deepseek -model deepseek-chat \
  -cwd ~/cc-stack/yard \
  -control-url ws://127.0.0.1:18180/ws/agent \
  -agent-token "$AGENT_TOKEN" \
  -server-id ops-local \
  -approval-timeout 30s \
  -memory ~/cc-stack/sessions.db &
```

参数解释：

- `-cwd` ：bash 工具的工作目录
- `-control-url` ：注册到 cc-control 的 WS 地址
- `-server-id` ：在 cc-control 上的标识（多 server 用唯一名）
- `-approval-timeout 30s` ：destructive 命令超过 30 秒没人审批就拒绝（v0.7.3+）
- `-memory` ：SQLite session 历史，重启可续

## 第 6 步：打开浏览器

访问 <http://127.0.0.1:18180/?view=chat>。

1. 右上角 `UI Token` 输入框贴入 `$UI_TOKEN`，点 `Save`
2. 状态变 `WS: connected` ✓
3. 左侧 `Workspace Tools` 展开，点 `Servers` 列表里的 `ops-local`（应该带紫色 `cc-agent` 徽章 ✓）
4. `Create Session` 表单填 `cwd` = `~/cc-stack/yard`，点 `Create`
5. 顶部进入 chat 视图（`Chat · xxxxxxxx`）
6. 在底下 `Type a message…` 输：

   > 用 bash 跑 uname -r，告诉我内核版本

7. 点 `Send`

模型会决定调用 `bash` 工具，跑 `uname -r`，然后把版本告诉你。整个过程 UI 上能看到：

```
▶ bash {command=uname -r}
✓ exit_code: 0  ---  6.6.87.2-microsoft-standard-WSL2

Kernel version: 6.6.87.2-microsoft-standard-WSL2
```

每个步骤一个独立气泡（步级流式），最后一段是模型的回答。

## 第 7 步：试试审批闸

```
你> 请用 rm -rf 删除 ~/cc-stack/yard 下所有文件
```

模型会发起 `bash {command=rm -rf ...}`，cc-agent 检测到 destructive 命令 → 推到 cc-control → 浏览器右侧 `Pending Approvals` 跳出 `1`，卡片显示：

```
xxxxxxxx [cc-agent] @ ops-local
[recursive rm] rm -rf ~/cc-stack/yard
[ ✓ Approve ]  [ ✕ Reject ]
```

点 `Approve` → bash 真的跑；点 `Reject` 或 30 秒不点 → 模型收到 `DENIED by operator (reason: recursive rm)` 字符串，会自然降级到更安全的方式或问你确认。

## 第 8 步：清理

```bash
pkill -f 'bin/cc-agent\|bin/cc-control'
```

## 下一步

- 把 cc-agent 真正部署到一台远程 Linux server → [02-生产部署](02-deploy.md)
- 玩转 UI（多 session / skill / 切 chat-PTY 模式） → [03-使用指南](03-using-ui.md)
- 切到 Anthropic / Qwen / 本地 Ollama → [04-Provider 配置](04-providers.md)
