# Router eval suite

A fixed dataset of prompts whose "right" skill is known by hand. Use it to
detect regressions in `LLMRouter` when changing the router prompt, the
candidate skill descriptions, or the model — and to compare
providers/models head-to-head.

## Run

```bash
# DeepSeek (cheap, default for the `cc-agent-eval` author):
go run ./cmd/cc-agent-eval \
  -provider deepseek \
  -base-url https://api.deepseek.com/v1

# Anthropic:
go run ./cmd/cc-agent-eval -provider anthropic -model claude-haiku-4-5

# OpenAI-compat (Qwen / vLLM / etc.):
go run ./cmd/cc-agent-eval \
  -provider openai-compat \
  -base-url https://your.endpoint/v1 \
  -model qwen2.5-72b-instruct
```

`-key-file` defaults to `~/.cc-agent-key` (one line, single API key). Falls
back to `CC_AGENT_API_KEY`, then provider-specific env (`ANTHROPIC_API_KEY`,
`OPENAI_API_KEY`).

## Useful flags

| Flag             | Default                       | Notes                                                       |
|------------------|-------------------------------|-------------------------------------------------------------|
| `-runs N`        | 1                             | Repeat each case N times — covers LLM non-determinism       |
| `-json`          | off                           | Emit one JSON report on stdout (for CI)                     |
| `-verbose`       | off                           | Print every individual call's verdict (stable + helpful)    |
| `-skills-dir`    | `tests/eval/skills`           | Where to load skill JSON files                              |
| `-cases`         | `tests/eval/cases.json`       | Test case file (see schema below)                           |
| `-timeout`       | `60s`                         | Per-call timeout                                            |

## Exit code

`0` if every case had at least one matching pick across its runs, `1`
otherwise. Suitable for CI gating; combine with `-runs 3` to require
robustness.

## Case schema

```json
{
  "prompt": "user message that the router will see",
  "expected": "skill-name" | null,
  "tags": ["clear-hit" | "ambiguous" | "no-match" | "en" | "zh" | ...],
  "notes": "optional rationale (not consumed by the harness)"
}
```

`expected: null` means "the router should pick no skill". The harness
treats `picked == ""` as a match.

## Adding skills / cases

1. Drop a new skill JSON into `tests/eval/skills/`. The harness loads
   every `.json` it finds — same format as the production skills loader.
2. Add cases to `cases.json` for the new domain. Aim for:
   - 3+ clear-hit prompts (different phrasings, mix of EN / ZH)
   - 1+ ambiguous prompt that overlaps another skill — pick the
     defensible answer in `expected` and explain why in `notes`
   - 1+ no-match prompt to make sure the router doesn't over-pick
3. Re-run the eval. If accuracy drops, look at the failing cases to
   decide whether the skill description needs sharpening, the case is
   genuinely ambiguous, or the router prompt needs adjusting.

## Cache stability check

The router itself is built to be cache-friendly: same skill set →
byte-identical `req.System` across calls, so DeepSeek's KV cache hits on
all but the per-turn user input. If you change the skill descriptions,
the cache is correctly invalidated — but the router behavior should
still pass the eval.
