# MCP Mode — Native IDE Integration

Instead of running shell commands, your IDE agent can connect to `llmgw` as an MCP (Model Context Protocol) server. This exposes gateway management as native tools directly inside your IDE — no bash required.

## Starting the MCP Server

```bash
llmgw mcp-serve
```

This starts a stdio MCP server. Your IDE connects to it and registers the available tools automatically.

## Available Native Tools

Once connected, the following tools are available directly in your IDE's tool registry:

| Tool | What It Does |
|---|---|
| `get_tenants()` | List all tenants |
| `create_tenant(name, tier)` | Create a new tenant |
| `topup_tenant(tenant_name, amount_usd)` | Add credits — **requires user approval** |
| `get_plugins()` | List available plugins and their schemas |
| `bind_plugin(plugin_name, scope_level, config, priority)` | Bind a plugin — **requires user approval** |
| `provision_project_env(tenant_name, key_name, budget)` | Full project setup in one call |

## Human-in-the-Loop (HITL) Approval

The two operations that affect money or security policy — `topup_tenant` and `bind_plugin` — will pause and display an approval prompt in your IDE before executing.

You will see something like:

```
Approve topping up $50 to tenant 'my-app'?
[Approve] [Reject]
```

The agent cannot proceed until you explicitly click **Approve**. If you click **Reject** (or dismiss the prompt), the operation is cancelled and the agent receives an error response — it will not retry silently.

This is an intentional hard gate, not a soft warning.

## Connecting in Cursor / Claude Code / Antigravity

Add the server to your IDE's MCP config. For most IDEs this is a JSON config file:

```json
{
  "mcpServers": {
    "llmgw": {
      "command": "llmgw",
      "args": ["mcp-serve"]
    }
  }
}
```

The gateway URL and admin key are read from your existing `llmgw config` (set via `llmgw config set` beforehand).

## When to Use MCP Mode vs Shell Commands

Use **MCP mode** when:
- You want the agent to have real-time, conversational access to gateway management
- You want HITL approval prompts built into the IDE UI
- The agent is running in a long-lived session (e.g., Cursor background agent)

Use **shell commands** (`llmgw --json ...`) when:
- You're scripting or automating gateway setup in CI/CD
- You want to chain commands with `jq` or shell scripting
- You're running in a headless environment where MCP clients aren't available
