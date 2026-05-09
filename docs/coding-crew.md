# CC = Coding Crew

This document records what `cc-*` stands for in this repository and the shared vocabulary we use when writing docs, commit messages, and UI copy.

## Why "Coding Crew"

The repo originally used `cc-` because it grew out of work around Claude Code. As of v0.8.0 the platform is no longer tied to a single CLI: it can host **Claude Code, Codex, Gemini CLI, OpenCode, and any agent you build yourself**. To match the new scope, `cc-` is reinterpreted as **Coding Crew**.

The metaphor: you (the operator) are the captain. The agents — third-party CLIs and ones you write yourself — are the crew. The control plane is the helm. Sessions are voyages. Skills are the rigging the crew can pick up and use.

This is a positioning change only. No directories, modules, binaries, or APIs are renamed.

## Vocabulary

Use these terms when writing prose. Code identifiers stay as they are.

| Concept | Term to prefer | Maps to |
|---|---|---|
| The product as a whole | the **Coding Crew** platform | `agent-control` |
| The control plane | the **helm** / control plane | `cc-control` |
| A coding agent under the platform | a **crew member** / agent | `cc-agent`, worker process |
| A user-built agent | **your own crew** | custom worker / runtime |
| A live session with one agent | a **voyage** / session | `session_id` |
| A reusable capability bundle | a **skill** (no rename needed) | skill marketplace |
| The browser UI | the **bridge** (informal) / web UI | `cc-web` |

You don't have to use the nautical terms in technical reference docs (`api.md`, `architecture.md`) — clarity beats theme. But for marketing copy, release notes, and UI microcopy, leaning into the metaphor keeps the brand coherent.

## Slogan

> **Code with your Crew.**

Short form for hero sections, GitHub social cards, and slide decks. Avoid stacking it next to other slogans on the same page.

## What this is *not*

- **Not a rename.** `cc-control` does not become `coding-crew-control`. Module paths, import paths, binary names, environment variables (`CC_CLAUDE_CMD` etc.), and CLI flags are unchanged.
- **Not a license to thematically rewrite existing docs.** Only update positioning copy when you're already editing that section for another reason.
- **Not a replacement for the formal product name.** The repository and product name remain "Agent Control"; "Coding Crew" is the meaning behind the `cc-` prefix and the brand voice we use when describing what the platform *feels* like to use.
