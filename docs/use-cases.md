# 使用场景

以下为典型使用场景及对应文档入口。

## 本地开发与联调

在本机同时跑 cc-control、cc-agent 和 Web UI，快速验证功能或调试。

- **步骤**：[快速上手](getting-started.md)（依赖 [README Quick Start](../README.md#quick-start)）
- **Token**：用 Admin 创建 Tenant，再在 Tenant 页生成 UI + Agent Token
- **Chat 模式**：README 中 [Chat Mode Quick Start](../README.md#chat-mode-quick-start) 配置 `-chat-worker`（如 cc-chat-echo / cc-chat-claude）

## 多租户 / 团队共用控制面

一个 cc-control 服务多团队，每个租户独立 Token、独立 Agent 与会话。

- **模型说明**：[架构 - 认证与租户隔离](architecture.md#3-认证与租户隔离)
- **Token 流程**：[API - Token 与鉴权](api.md) 及 [README - Token Model](../README.md#token-model-latest)
- **管理入口**：`/admin` 创建 Tenant Token，`/tenant` 由租户自助签发 UI/Agent Token

## PTY 终端 + Chat 统一会话

在同一会话内切换终端（PTY）与聊天（Chat），共用同一 `session_id`，支持 Claude 会话恢复。

- **产品行为**：README [Unified Session ID](../README.md#unified-session-id)、[Chat Mode Quick Start](../README.md#chat-mode-quick-start)
- **权限与多聊天**：[聊天模式与权限](chat-mode-permissions.md)、[多聊天 Bundle 使用](multichat-bundle-usage.md)

## 公网或生产部署

将 control 暴露到公网、配置 TLS、多台 Agent 或 Cloudflare Tunnel 等。

- **入口**：[公网部署总览](deploy-public-server.md)
- **Agent 常驻**：[后台部署指南](deploy-public-server/04-agent-background.md)（Linux systemd / Windows NSSM）

## 原生客户端（macOS / iOS）

使用 AgentControl 原生 App 接入同一控制面，协议与 Web 一致。

- **部署与运维**：[运维与升级](deploy-public-server/03-operations.md) 中客户端接入说明
- **项目位置**：`app/AgentControlMac/`

## 服务器运维（cc-agent · v0.7.0+）

把自研 cc-agent 部署到一台 Linux 服务器，让它自主驱动 LLM 调用 bash / read /
grep / sysinfo / proclist / logtail 等内置工具完成日常运维：日志巡检、进程
排查、磁盘清理、配置变更、系统状态汇报等。**destructive 命令（rm -rf /
mkfs / systemctl stop / shutdown / ...）默认在 Web/iOS/Win UI 上等运维点
Approve 才执行**，超时自动拒绝。

适合场景：

- 单机或多台服务器，每台跑一个 cc-agent，UI 上一键切换执行环境
- nginx / redis / postgresql 等服务的日志诊断 + 重启决策
- 定期巡检（磁盘、内存、进程、登录历史）+ 自动报告
- "我想问一下生产服务器的内核版本和负载" 这种轻量交互
- 通过 `:reflect <name>` 把成功的运维流程蒸馏成可复用 skill，下次秒启

入门读这里：

- **教程**：[Tutorial 入门](tutorial/README.md) → 5 分钟跑通本地 + 生产部署
- **模块文档**：[`cc-agent/README.md`](../cc-agent/README.md)
- **审批闸**：[v0.7.1 release notes](v0.7.1-release-notes.md)（UI 路由）+
  [v0.7.3](v0.7.3-release-notes.md)（超时可配置 `-approval-timeout 30s/30m/1h`）
- **Provider 配置**：[tutorial/04-providers.md](tutorial/04-providers.md)（DeepSeek / Anthropic / Qwen / 本地 Ollama）

> 与 cc-proxy（包外部 Claude Code / Codex / Gemini CLI）的区别：cc-agent 自己
> 跑 LLM 主循环，不依赖外部 CLI；UI 上以紫色 `cc-agent` 徽章区分。一台 server
> 装哪个看你想要什么——要复用 Claude Code 全套就用 cc-proxy，要更可控的
> server-ops agent 就用 cc-agent。两者协议在 cc-control 处统一，UI 一致。

## 生物信息学分析（cc-agent + bash 工具链）

cc-agent 的 bash 工具就是 `sh -c` 包装，**任何已 `apt install` / `conda
activate` 好的生信工具都能直接调用**：BWA / samtools / bcftools / STAR /
hisat2 / minimap2 / bedtools / vcftools / Snakemake / Nextflow ...

典型 chat 会话流（实测可行）：

```
你> 用 bwa 把 sample.fastq.gz 比对到 hg38，输出排序后的 bam

▶ bash {command=which bwa samtools && ls /data/ref/hg38*}
✓ /usr/bin/bwa /usr/bin/samtools / hg38.fa + 索引齐全
▶ bash {command=ls -la sample.fastq.gz && zcat sample.fastq.gz | head -4}
✓ 4.5G / 看起来是 Illumina 150bp PE
▶ bash {command=bwa mem -t 16 hg38.fa sample.fastq.gz | samtools sort -@8 -o sample.bam -, timeout_sec=600}
✓ 95.3% mapped, 8min42s
assistant> sample.bam (8.7G) 已生成，比对率 95.3%（健康）。建议下一步...
```

适合场景：

- **探索性分析**：模型自己 plan + 多步 ReAct（先 `which` 检查环境 → `ls`
  看数据 → 跑流程 → 读 stderr 修正 → 总结）
- **小规模流程**：单样本 align、call variant、做 QC 报告
- **脚本生成**：让模型写 `analysis.py` / `analysis.R` 然后跑
- **流程触发 + 监控**：`nextflow run ... -bg` + tail `.nextflow.log` + 主动
  检查 `ps`
- **跨 server 调度**：cc-control 一套 UI 管多台计算节点

约束（写在 `cc-agent/internal/tools/bash.go`）：

| 约束 | 当前值 | 影响 |
|---|---|---|
| 单条 bash 命令超时 | `timeout_sec` 默认 60s，**最大 600s = 10min** | 长跑任务（whole-genome、大样本）需用 `nohup ... & echo $!` 模式拆分 |
| bash stdout/stderr 各自上限 | 64 KB | 大输出截断，但 exit_code + tail 可见 |
| read 工具读文件 | 256 KB / 2000 行 | BAM/big FASTQ 用 `bash zcat \| head` |
| 命令运行期间无流式输出 | — | 模型只在命令结束后看到 stdout |

长跑任务建议 pattern（模型会主动用，也可以 `:reflect bioinfo-bg-job`
蒸馏成 skill 重用）：

```bash
nohup <slow-cmd> > job.log 2>&1 & echo $! > job.pid
# 然后让模型轮询：
ps -p $(cat job.pid) > /dev/null && tail -10 job.log
```

推荐配置：

```bash
./cc-agent \
  -provider anthropic -model claude-sonnet-4-6 \   # bioinfo 命令行复杂，Claude 在领域细节上更稳
  -cwd /data/projects \
  -approval-timeout 5m \
  -memory /var/lib/cc-agent/sessions.db \
  -skills-dir /etc/cc-agent/skills.d \
  -control-url ws://your-control:18180/ws/agent \
  -agent-token <token> -server-id bioinfo-node-01
```

模型选型差异：

| Provider / Model | bioinfo 适用度 |
|---|---|
| Claude `claude-sonnet-4-6` | ✓ 推荐，参数最稳 |
| DeepSeek `deepseek-chat` | ✓ 便宜，简单流程 OK，复杂 RG 字符串偶尔少参数 |
| Ollama `qwen2.5:32b+` | 可用，速度看硬件 |
| Ollama < 14B | ⚠ tool calling 不稳，不推荐 |

不能做（当前阶段）：

- GUI / Jupyter 交互（没有）
- R / Python 交互式 REPL（每次 bash 是无状态的；解决：让模型写脚本，再
  `python script.py`）
- bash 工具自带的并行调度（`xargs -P` 可以但截断会丢信息；推荐用 Snakemake/
  Nextflow 做并行，cc-agent 做 orchestrator）

详细架构 + skills 写法见 [`cc-agent/README.md`](../cc-agent/README.md)。
