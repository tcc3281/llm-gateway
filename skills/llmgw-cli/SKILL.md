---
name: llmgw-cli
description: How to help developers configure and manage an LLM Gateway deployment using the llmgw CLI. Use this skill when the user wants to provision API keys, add team members, set spending limits, configure guardrail plugins, set up virtual model routing aliases, or troubleshoot gateway errors. Trigger on any mention of: llmgw, llm-gateway config, api key provisioning, model routing, budget limits, rate limits, prompt-guard, or tenant setup.
---

# llmgw CLI — Agent Skill Entry Point

`llmgw` is the command-line tool for managing an LLM Gateway deployment. It lets developers configure tenants, API keys, plugins, model routing, and access control—all from the terminal or through an IDE agent.

## Installation

Users install `llmgw` as a native binary. They do **not** use `uv` or Python directly:

```bash
# Debian/Ubuntu — from release .deb package
sudo dpkg -i llmgw_*.deb

# Or from release zip
sudo mv llmgw-linux-amd64 /usr/local/bin/llmgw && chmod +x /usr/local/bin/llmgw

# macOS / Windows — download binary from GitHub Releases
```

## First Things First

Before any command works, configure the gateway URL and admin key:

```bash
llmgw config set --url http://localhost:8081 --key sk-admin-secret
```

Verify connectivity:

```bash
llmgw health
```

---

## Reference Files — Read These for Specific Tasks

| Situation | Read This File |
|---|---|
| Setting up a project, provisioning a key from scratch | [references/quickstart.md](references/quickstart.md) |
| Common day-to-day tasks (add user, set limits, routing, plugins) | [references/usecases.md](references/usecases.md) |
| Command returned an error, something is broken | [references/error-handling.md](references/error-handling.md) |
| IDE agent wants native MCP tool integration (not shell) | [references/mcp-mode.md](references/mcp-mode.md) |

---

## Three Rules That Matter

**1. `--json` flag for scripting / agent use**
Human-readable tables are fine for reading, but for parsing output or chaining commands use `--json`:
```bash
llmgw --json tenant list
```

**2. `--yes` for destructive operations**
Delete, revoke, topup all require explicit confirmation. Without `--yes` the command asks interactively—or returns `CONFIRMATION_REQUIRED` if run non-interactively:
```bash
llmgw --json key revoke --id my-key --yes
```

**3. `env-sync` for full project setup in one shot**
Instead of running 3–4 separate create commands, use the built-in macro:
```bash
llmgw env-sync --tenant my-project --key-name default --budget 20
```
