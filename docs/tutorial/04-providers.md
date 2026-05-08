# 04 · Provider 配置

cc-agent 通过统一的 LLM provider 抽象支持多家厂商。本篇覆盖最常用的几种。

## 总览

| Provider | 默认 model | 适用场景 |
|---|---|---|
| `anthropic` | `claude-sonnet-4-6` | 复杂推理、长上下文，价格较高 |
| `deepseek` | `deepseek-chat` | 性价比高，中文友好，国内访问稳 |
| `openai` | `gpt-4o-mini` | 兼容性最广 |
| `qwen` | `qwen-plus` | 阿里云，国内零延迟 |
| `openai-compat` | 自定义 | vLLM / Ollama / llama.cpp 自建 |

切换三种方式（按优先级从高到低）：CLI flag > env > JSON config。

## DeepSeek（推荐入门）

```bash
echo 'sk-xxxxxxxx' > ~/.cc-agent-key && chmod 600 ~/.cc-agent-key

CC_AGENT_API_KEY="$(cat ~/.cc-agent-key)" \
CC_AGENT_BASE_URL="https://api.deepseek.com" \
./cc-agent -provider deepseek -model deepseek-chat -cwd /var/log
```

### 模型选哪个

| 模型 | 推荐 | 说明 |
|---|---|---|
| `deepseek-chat` | ✓ | V3 + 函数调用，秒级响应，1m 内出答案 |
| `deepseek-v4-pro` | 复杂任务 | thinking 模式（v0.7.0+ cc-agent 自动 reasoning_content 回传），单 turn 1-3min，更准确 |
| `deepseek-reasoner` | 长推理 | R1 系列，深度思考，分钟级 |

### 价格（2026/05）

|  | input  | output |
|---|---|---|
| deepseek-chat | $0.14 / 1M tokens | $0.28 / 1M tokens |

跑一次 ReAct 循环（5-10 个 tool 调用）通常 ~5K input + ~500 output tokens，约 0.001 美元。**便宜到忽略不计。**

## Anthropic Claude

```bash
echo 'sk-ant-xxxx' > ~/.cc-agent-key && chmod 600 ~/.cc-agent-key

CC_AGENT_API_KEY="$(cat ~/.cc-agent-key)" \
./cc-agent -provider anthropic -model claude-sonnet-4-6 -cwd /var/log
```

Provider=`anthropic` 时不用设 `CC_AGENT_BASE_URL`。

### 模型选哪个

| 模型 | 推荐 | 说明 |
|---|---|---|
| `claude-haiku-4-5` | 简单运维 | 最快、最便宜，适合日志巡检 |
| `claude-sonnet-4-6` | ✓ 平衡 | 默认值，速度和准确度都不错 |
| `claude-opus-4-7` | 深度调试 | 最强，价格也最高 |

> **注意**：tool_use 协议是 Anthropic 原生的，cc-agent 直接走 Messages API（不经过 OpenAI-compat 转换），所以效果最稳。

## 通义 Qwen（DashScope）

```bash
CC_AGENT_API_KEY="$(cat ~/.cc-agent-key)" \
CC_AGENT_BASE_URL="https://dashscope.aliyuncs.com/compatible-mode/v1" \
./cc-agent -provider openai-compat -model qwen-plus -cwd /var/log
```

阿里 DashScope 提供 OpenAI 兼容接口。模型选 `qwen-plus` / `qwen-max` / `qwen-turbo`。

> 国内服务器直连阿里云零延迟，比 DeepSeek 还快。但 tool_use 准确度略低于 DeepSeek，复杂任务可能需要重试。

## 本地 Ollama（零成本）

```bash
# 先起 Ollama
ollama serve &
ollama pull qwen2.5:14b

# cc-agent 接到本地
CC_AGENT_API_KEY="dummy" \
CC_AGENT_BASE_URL="http://localhost:11434/v1" \
./cc-agent -provider openai-compat -model qwen2.5:14b -cwd /var/log
```

### 模型推荐

| 模型 | 内存 | tool calling 准确度 |
|---|---|---|
| `qwen2.5:7b` | ~6GB | ⚠ 容易乱叫工具，不推荐 |
| `qwen2.5:14b` | ~10GB | 可用，简单任务 OK |
| `qwen2.5:32b` | ~20GB | ✓ 推荐起步线 |
| `qwen2.5:72b` | ~48GB | 接近 DeepSeek-chat |
| `llama3.3:70b` | ~40GB | tool calling 弱于 qwen，不推荐 |

> **小模型（< 14B）的 tool_use 准确率在生产环境不可靠**。本地模型建议 14B 起步，工程化场景 32B 起步。

## OpenAI 官方

```bash
echo 'sk-xxx' > ~/.cc-agent-key

CC_AGENT_API_KEY="$(cat ~/.cc-agent-key)" \
./cc-agent -provider openai -model gpt-4o-mini -cwd /var/log
```

不用设 `CC_AGENT_BASE_URL`（默认 `https://api.openai.com/v1`）。

## 自建 vLLM / SGLang

跟 Ollama 一样走 `openai-compat`：

```bash
CC_AGENT_API_KEY="dummy-or-your-token" \
CC_AGENT_BASE_URL="http://your-vllm:8000/v1" \
./cc-agent -provider openai-compat -model my-model -cwd /var/log
```

确保 vLLM 启动时开了 tool calling 支持（`--enable-auto-tool-choice --tool-call-parser hermes` 之类）。

## 在配置文件里

`/etc/cc-agent/config.json`：

```json
{
  "provider": "deepseek",
  "model": "deepseek-chat",
  "base_url": "https://api.deepseek.com",
  "api_key": "",
  "timeout": "300s",
  "cwd": "/var/log",
  "memory_path": "/var/lib/cc-agent/sessions.db"
}
```

> ⚠ `api_key` 字段建议留空，让 cc-agent 从 `CC_AGENT_API_KEY` env 读，避免明文落盘。

启动：

```bash
CC_AGENT_API_KEY="$(cat /etc/cc-agent/api-key)" \
./cc-agent -config /etc/cc-agent/config.json
```

## reasoning_content（思考模式）

DeepSeek `deepseek-v4-pro` 和 Qwen 的 `qwen-plus-thinking` 等会在响应里返回 `reasoning_content` 字段（思维链）。**cc-agent 自动捕获并在下一轮回传**，让模型的思维链跨工具调用保持一致——你不需要做任何配置。

只要你用的 model 支持 thinking，cc-agent 就会自动跑 reasoning round-trip。

## 切 provider 不影响 session

`-memory` 持久化的是 LLM-中立的 message 历史。换 provider 重启后用同一个 `-session` ID 接续，模型可以从前面的对话继续——但**不要在同一个 session 内频繁切**，因为不同模型的 tool_use 风格有差异，可能导致历史里的 tool_call 在新模型里被忽略。

## 性能对比（粗估）

跑同一个任务"读 /etc/os-release，跑 uname -r，跑 uptime，总结系统"：

| Provider / Model | 第 1 个工具调度 | 总耗时 | 备注 |
|---|---|---|---|
| Anthropic claude-sonnet-4-6 | 2-3s | 6-8s | 最稳 |
| DeepSeek deepseek-chat | 1-2s | 4-6s | 最快 |
| DeepSeek deepseek-v4-pro (thinking) | 30-90s | 1-3min | 准确度 +，速度 - |
| Qwen qwen-plus | 2-4s | 6-10s | 国内零延迟 |
| Ollama qwen2.5:14b | 5-10s | 20-40s | 取决于硬件 |
| Ollama qwen2.5:32b | 10-15s | 40-80s | RTX4090 测试 |

## 下一步

- 写 skill 让模型把成功 session 蒸馏成可复用的 prompt → [`cc-agent/README.md` Skills 节](../../cc-agent/README.md#skills)
- 报错排查 → [troubleshooting](troubleshooting.md)
