# 后台部署指南

将 **cc-agent** 作为后台服务长期运行（连接已有 cc-control）。

> 控制面部署与 Token 创建见 [公网部署总览](../deploy-public-server.md) 及 01/02 子文档。

## 编译

在开发机上为目标平台编译 cc-agent（及可选 chat worker）：

```bash
# Linux amd64
cd cc-agent
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cc-agent ./cmd/cc-agent
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cc-chat-claude ./cmd/cc-chat-claude

# Windows amd64
cd cc-agent
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o cc-agent.exe ./cmd/cc-agent
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o cc-chat-claude.exe ./cmd/cc-chat-claude
```

## Linux

### 方式一：systemd（推荐）

开机自启、崩溃自动重启、日志集成 journalctl。

`/etc/systemd/system/cc-agent.service`：

```ini
[Unit]
Description=CC Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=deploy
Group=deploy
WorkingDirectory=/opt/cc-agent
ExecStart=/opt/cc-agent/cc-agent \
  -control-url ws://127.0.0.1:18080/ws/agent \
  -agent-token ${AGENT_TOKEN} \
  -server-id ${SERVER_ID} \
  -allow-root /home/deploy/repos \
  -claude-path /usr/local/bin/claude \
  -chat-worker /opt/cc-agent/cc-chat-claude
EnvironmentFile=/opt/cc-agent/.env
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

`/opt/cc-agent/.env`（权限 600）：

```bash
AGENT_TOKEN=<AGENT_TOKEN>
SERVER_ID=srv-01
```

启用：

```bash
systemctl daemon-reload
systemctl enable --now cc-agent
```

常用操作：

```bash
systemctl status cc-agent
journalctl -u cc-agent -f
systemctl restart cc-agent
systemctl stop cc-agent
```

### 方式二：nohup（快速临时）

```bash
nohup /opt/cc-agent/cc-agent \
  -control-url ws://127.0.0.1:18080/ws/agent \
  -agent-token "$AGENT_TOKEN" \
  -server-id srv-01 \
  -allow-root /home/deploy/repos \
  -claude-path /usr/local/bin/claude \
  > /opt/cc-agent/agent.log 2>&1 &
```

缺点：不会开机自启，崩溃不自动重启。

### 方式三：tmux / screen

```bash
tmux new -d -s cc-agent '/opt/cc-agent/cc-agent -control-url ws://127.0.0.1:18080/ws/agent -agent-token "$AGENT_TOKEN" -server-id srv-01 -allow-root /home/deploy/repos -claude-path /usr/local/bin/claude'
tmux attach -t cc-agent
```

## Windows

### 方式一：NSSM 注册为 Windows 服务（推荐）

[NSSM](https://nssm.cc/) 可将任意 exe 注册为 Windows 服务，支持自动重启和开机自启。

```powershell
# 安装：scoop install nssm 或 choco install nssm

nssm install cc-agent "C:\cc\cc-agent.exe"
nssm set cc-agent AppParameters "-control-url ws://127.0.0.1:18080/ws/agent -agent-token %AGENT_TOKEN% -server-id srv-win -allow-root C:\projects -claude-path C:\path\to\claude.exe -chat-worker C:\cc\cc-chat-claude.exe"
nssm set cc-agent AppDirectory "C:\cc"
nssm set cc-agent AppStdout "C:\cc\agent-stdout.log"
nssm set cc-agent AppStderr "C:\cc\agent-err.log"

nssm start cc-agent
```

管理：`nssm status cc-agent`、`nssm restart cc-agent`、`nssm stop cc-agent`、`nssm remove cc-agent`（需先停止）。

### 方式二：Task Scheduler（任务计划程序）

```powershell
$action = New-ScheduledTaskAction `
  -Execute "C:\cc\cc-agent.exe" `
  -Argument "-control-url ws://127.0.0.1:18080/ws/agent -agent-token YOUR_AGENT_TOKEN -server-id srv-win -allow-root C:\projects -claude-path C:\path\to\claude.exe" `
  -WorkingDirectory "C:\cc"
$trigger = New-ScheduledTaskTrigger -AtStartup
$settings = New-ScheduledTaskSettingsSet -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
Register-ScheduledTask -TaskName "cc-agent" -Action $action -Trigger $trigger -Settings $settings -User "SYSTEM" -RunLevel Highest
```

管理：`Start-ScheduledTask -TaskName "cc-agent"`、`Stop-ScheduledTask -TaskName "cc-agent"`、`Unregister-ScheduledTask -TaskName "cc-agent"`。

### 方式三：PowerShell 后台进程（快速临时）

```powershell
Start-Process -NoNewWindow -FilePath "C:\cc\cc-agent.exe" `
  -ArgumentList "-control-url ws://127.0.0.1:18080/ws/agent -agent-token $env:AGENT_TOKEN -server-id srv-win -allow-root C:\projects -claude-path C:\path\to\claude.exe" `
  -RedirectStandardOutput "C:\cc\agent.log" `
  -RedirectStandardError "C:\cc\agent-err.log"
```

关闭终端后进程仍在；不开机自启、不自动重启。

## 推荐做法

| 场景 | Linux | Windows |
|------|-------|---------|
| 生产 / 长期运行 | systemd | NSSM 或 Task Scheduler |
| 临时测试 | nohup / tmux | PowerShell Start-Process |
| 需要看实时输出 | tmux attach | 前台窗口直接运行 |

> cc-agent 为出站连接 control，本机无需开放入站端口；防火墙仅需在 **运行 cc-control 的那台机器** 上配置。
