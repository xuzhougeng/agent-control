# Use Cases

Pick the section that matches your goal. Each one points to the relevant deployment depth (stand-alone cc-agent, cc-agent + cc-control, cc-proxy, or hosted).

## Standalone automation on your laptop

Run `cc-agent` as a single binary against your favourite LLM. No control plane, no tokens. You drive it from the REPL.

- **Setup**: [Getting Started · step 1](getting-started.md#1--run-cc-agent-locally-single-binary) (English) or [tutorial/01-quickstart](tutorial/01-quickstart.md) (Chinese, full sample).
- **What it's good for**: trying out Agent Control, embedding `cc-agent` into a one-shot script, or just chatting with an LLM that can actually run `bash`.

## Server operations (cc-agent · v0.7.0+)

Drop `cc-agent` on a Linux server and let it drive bash / read / grep / sysinfo / proclist / logtail to do day-2 ops: log triage, process inspection, disk cleanup, config audits, status reports. **Destructive commands** (`rm -rf`, `mkfs`, `systemctl stop`, `shutdown`, ...) wait for an Approve in the UI; auto-reject after timeout.

Good fits:

- One or many servers, each with a `cc-agent`; the UI switches between them.
- Diagnosing nginx / redis / postgres logs and deciding whether to restart.
- Scheduled health checks (disk, memory, processes, login history) and reports.
- Light "what's the kernel version and load on prod?" interactions.
- Distill a successful run into a reusable skill via `:reflect <name>`.

Pointers:

- **Tutorial**: [tutorial/](tutorial/) — local 5-min run → production deploy → UI.
- **Module docs**: [`cc-agent/README.md`](https://github.com/xuzhougeng/agent-control/blob/main/cc-agent/README.md).
- **Approval gate**: [v0.7.1 release notes](v0.7.1-release-notes.md) (UI routing) + [v0.7.3](v0.7.3-release-notes.md) (configurable `-approval-timeout`).
- **Provider config**: [tutorial/04-providers.md](tutorial/04-providers.md) — DeepSeek / Anthropic / Qwen / local Ollama.

## Bioinformatics analysis (cc-agent + the bash toolchain)

The `bash` tool is just `sh -c`. **Anything you've already `apt install`-ed or `conda activate`-d is callable**: BWA, samtools, bcftools, STAR, hisat2, minimap2, bedtools, vcftools, Snakemake, Nextflow ...

Realistic chat flow:

```
you> Map sample.fastq.gz to hg38 with bwa, sorted bam out.

▶ bash {command=which bwa samtools && ls /data/ref/hg38*}
✓ /usr/bin/bwa /usr/bin/samtools / hg38.fa + indexes ok
▶ bash {command=ls -la sample.fastq.gz && zcat sample.fastq.gz | head -4}
✓ 4.5G / looks like Illumina 150bp PE
▶ bash {command=bwa mem -t 16 hg38.fa sample.fastq.gz | samtools sort -@8 -o sample.bam -, timeout_sec=600}
✓ 95.3% mapped, 8min42s
assistant> sample.bam (8.7G) ready, 95.3% mapped (healthy). Suggested next step ...
```

Good fits:

- **Exploratory analysis**: model self-plans multi-step ReAct (`which` → `ls` → run → read stderr → fix → summarise).
- **Small flows**: single-sample align, variant call, QC report.
- **Script generation**: have the model write `analysis.py` / `analysis.R` then run it.
- **Trigger + monitor**: `nextflow run ... -bg` plus polling `tail .nextflow.log` and `ps`.
- **Multi-server scheduling**: one cc-control UI, many compute nodes.

Constraints (defined in `cc-agent/internal/tools/bash.go`):

| Constraint | Default | Impact |
|---|---|---|
| Per-bash command timeout | `timeout_sec` default 60s, **max 600s = 10 min** | Long jobs (whole-genome, large samples) need `nohup ... & echo $!` patterns |
| stdout / stderr cap | 64 KB each | Big outputs are truncated, but exit_code + tail are kept |
| `read` tool file size | 256 KB / 2000 lines | Use `bash zcat \| head` for BAM / large FASTQ |
| No streaming during a command | — | The model only sees stdout when the command finishes |

Recommended config:

```bash
./cc-agent \
  -provider anthropic -model claude-sonnet-4-6 \   # Claude is the most stable on bioinfo flag minutiae
  -cwd /data/projects \
  -approval-timeout 5m \
  -memory /var/lib/cc-agent/sessions.db \
  -skills-dir /etc/cc-agent/skills.d \
  -control-url ws://your-control:18180/ws/agent \
  -agent-token <token> -server-id bioinfo-node-01
```

Model selection:

| Provider / Model | Bioinfo fit |
|---|---|
| Claude `claude-sonnet-4-6` | ✓ Recommended, most stable on flags |
| DeepSeek `deepseek-chat` | ✓ Cheap, fine for simple flows; occasionally drops args on complex RG strings |
| Ollama `qwen2.5:32b+` | OK; depends on hardware |
| Ollama < 14B | ⚠ Tool calling unreliable; not recommended |

Out of scope for now:

- GUI / Jupyter interactivity.
- Stateful R / Python REPL (each bash call is fresh — have the model write a script, then run it).
- Built-in parallel scheduling. `xargs -P` works but truncated output loses information; use Snakemake / Nextflow for parallelism and let cc-agent be the orchestrator.

## Wrapping Claude Code / Codex / Gemini (cc-proxy)

Already invested in `claude`, `codex`, or `gemini` CLI? Run `cc-proxy` instead of `cc-agent` on that node — it surfaces the external CLI as a regular session in the same UI (Browser / iOS / macOS / Windows). cc-agent and cc-proxy live in the same control plane and the UI handles both.

- **Setup**: [Getting Started · step 3](getting-started.md#3--cc-proxy--wrap-an-external-cli-agent).
- **PTY + chat unified session**: [Chat mode permissions](chat-mode-permissions.md), [Multichat bundle usage](multichat-bundle-usage.md).
- **Chat mode quick start**: see [README · Chat Mode](https://github.com/xuzhougeng/agent-control#chat-mode-quick-start).

## Multi-tenant / shared control plane

One `cc-control` serving multiple teams. Each tenant has its own tokens, agents, and sessions.

- **Model**: [Architecture · auth & tenant isolation](architecture.html#3-认证与租户隔离).
- **Tokens**: [API · token & auth](api.md), [README · Token Model](https://github.com/xuzhougeng/agent-control#token-model-latest).
- **Admin entry**: `/admin` mints tenant tokens; `/tenant` lets the tenant self-serve UI / Agent tokens.

## Public-internet / production deploy

Expose `cc-control` to the internet, terminate TLS, run multiple agents, or front it with a Cloudflare Tunnel.

- **Overview**: [Public-server deployment](deploy-public-server.md).
- **Long-running agent**: [Background agent guide](deploy-public-server/04-agent-background.md) (Linux systemd / Windows NSSM).

## Native macOS / iOS clients

Use the AgentControl native app against the same control plane (same protocol as the web UI).

- **Operations notes**: [Operations & upgrades](deploy-public-server/03-operations.md).
- **Project location**: `app/AgentControlMac/`.
