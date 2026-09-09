---
name: llmgw-cli
description: How to use the LLM Gateway CLI (llmgw) to configure tenants, api keys, plugins, models, and IAM groups. Make sure to use this skill whenever the user asks to provision gateway resources, set up project API keys, bind guardrail plugins, or configure the LLM Gateway environment.
---

# llmgw: Agent Cheatsheet & MCP Guide

This document is the official skill/rule file for IDE AI Agents (Cursor, Windsurf, Claude Code, Antigravity) that interact with the `llmgw` (LLM Gateway) CLI.

## Overview
`llmgw` is the infrastructure management CLI for the LLM Gateway.
**As an AI Agent, you are the primary consumer of this CLI.** Do not rely on rich tables; always use structured JSON outputs and automated workflows.

## Golden Rules for Agents

1. **Always use the `--json` flag:**
   When running standard commands, prepend the flag immediately after the base command:
   ```bash
   uv run --project cli llmgw --json tenant list
   uv run --project cli llmgw --json key create --user-id "uuid" --name "my-key"
   ```
   *Never parse standard human-readable tables.*

2. **Destructive Commands REQUIRE `--yes`:**
   Any command that deletes, revokes, or consumes resources has a hard gate. If you do not pass `--yes`, the command will abort with a `CONFIRMATION_REQUIRED` error.
   Example: `uv run --project cli llmgw --json key revoke --id "my-key" --yes`

3. **Idempotency & Upsert:**
   When creating resources (like a Tenant or API Key), anticipate HTTP 400 (Already Exists). Handle gracefully by using `list` or `get` to retrieve the existing entity's ID instead of failing.

4. **Use the `env-sync` Macro:**
   If asked to "provision an API key" or "connect this project to the gateway", DO NOT run individual `tenant create`, `user create`, `key create` commands. Instead, use the built-in macro command that handles idempotency safely and writes the `.env` file directly:
   ```bash
   uv run --project cli llmgw --json env-sync --tenant "project-name" --key-name "default" --budget 10
   ```

## Quick Start Configuration
If you receive authentication errors (HTTP 401), or need to set up the CLI for the first time:
```bash
uv run --project cli llmgw config set --url "http://localhost:8081" --key "sk-admin-secret"
```

## Command Reference

### Tenant Management
- List tenants: `llmgw --json tenant list`
- Create tenant: `llmgw --json tenant create --name "tenant-1" --tier "pro"`
- Update tenant: `llmgw --json tenant update --id "tenant-uuid" --name "tenant-2" --tier "enterprise"`
- Delete tenant: `llmgw --json tenant delete --id "tenant-uuid" --yes`
- Add balance: `llmgw --json tenant topup --id "tenant-uuid" --amount 50.0 --yes`

### API Key & Model Policy Management
- List keys: `llmgw --json key list --user-id "user-uuid"`
- Create key: `llmgw --json key create --user-id "user-uuid" --name "dev-key" --budget 10 --rpm 60`
- Revoke key: `llmgw --json key revoke --id "key-uuid" --yes`
- Reactivate key: `llmgw --json key reactivate --id "key-uuid"`
- List model policies: `llmgw --json key model list --key-id "key-uuid"`
- Add allowlist rule: `llmgw --json key model add --key-id "key-uuid" --pattern "gpt-4.*" --allow`
- Add denylist rule: `llmgw --json key model add --key-id "key-uuid" --pattern "claude-*" --deny`
- Remove rule: `llmgw --json key model delete --key-id "key-uuid" --policy-id "policy-uuid" --yes`

### Plugin & Guardrail Management
- List plugins catalog: `llmgw --json plugin list`
- Get plugin schema: `llmgw --json plugin get --name "prompt-guard"`
- List bindings: `llmgw --json plugin bindings --tenant "tenant-uuid"`
- Bind plugin: 
  ```bash
  llmgw --json plugin bind --plugin "prompt-guard" --scope "tenant" --scope-id "<tenant-uuid>" --config '{"reject_injection": true, "reject_pii": false}' --priority 10 --yes
  ```
- Unbind plugin: `llmgw --json plugin unbind --id "binding-uuid" --yes`

### Virtual Models (Aliases)
- List aliases: `llmgw --json alias list`
- Create alias: `llmgw --json alias create --name "fast-model" --target "gpt-4o-mini" --fallback "claude-3-haiku"`
- Delete alias: `llmgw --json alias delete --id "alias-uuid" --yes`

## Using the Built-In MCP Server
If your environment supports connecting to external MCP servers (e.g. `fastmcp` stdio), you do not need to use bash commands at all!
You can start the MCP server using:
```bash
uv run --project cli llmgw mcp-serve
```
This exposes Native Tools to your IDE for tenant, plugin, and environment provisioning workflows. Note that destructive tools natively trigger Human-in-the-Loop (HITL) pop-ups.

## Troubleshooting Error Codes
When `llmgw --json` encounters an error, it returns a structured JSON:
```json
{
  "status": "error",
  "code": "HTTP_404",
  "message": "Resource not found",
  "suggested_action": "Check parameters or permissions."
}
```
Use the `code` and `message` to self-correct your next action. Do not ask the user unless the credentials (X-Admin-API-Key) are missing or invalid (HTTP_401).
