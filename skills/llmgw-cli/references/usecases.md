# Common Use Cases

Real-world workflows for day-to-day gateway management.

---

## Use Case 1: Give a New Team Member Access

```bash
# Create the user
llmgw --json user create --email "alice@company.com" --name "Alice" --role "user"

# Create their API key (use the user ID from above)
llmgw --json key create --user-id "<alice-user-id>" --name "alice-dev" --budget 50 --rpm 120
```

Share the API key value and the gateway's public URL with Alice. She sets in her environment:

```bash
export OPENAI_API_BASE="http://gateway.internal:8080/v1"
export OPENAI_API_KEY="sk-..."
```

---

## Use Case 2: Set or Change Spending Limits on a Key

List keys to find the target:

```bash
llmgw --json key list
```

Update the budget or rate limit:

```bash
llmgw --json key update --id "<key-id>" --budget 100 --rpm 200
```

To set a hard expiry date:

```bash
llmgw --json key update --id "<key-id>" --expires-at "2025-12-31T23:59"
```

---

## Use Case 3: Restrict Which Models a Key Can Use

By default a key can call any model. Use allowlist rules to lock it down:

```bash
# Allow only GPT-4 variants
llmgw --json key model add --key-id "<key-id>" --pattern "gpt-4.*" --allow

# Or block a specific model
llmgw --json key model add --key-id "<key-id>" --pattern "gpt-4o" --deny
```

Review active rules:

```bash
llmgw --json key model list --key-id "<key-id>"
```

---

## Use Case 4: Enable a Security Plugin (Prompt Guard, Budget Control)

First, check what plugins are available and what config they expect:

```bash
llmgw --json plugin list
llmgw --json plugin get --name "prompt-guard"
```

The `plugin get` response shows the exact config schema. Then bind it:

```bash
# Apply prompt injection protection to a specific tenant
llmgw --json plugin bind \
  --plugin "prompt-guard" \
  --scope "tenant" \
  --scope-id "<tenant-id>" \
  --config '{"reject_injection": true, "reject_pii": false}' \
  --priority 10 \
  --yes

# Or apply globally (no --scope-id needed)
llmgw --json plugin bind \
  --plugin "budget-control" \
  --scope "global" \
  --config '{"hard_limit_usd": 500}' \
  --yes
```

Check active bindings:

```bash
llmgw --json plugin bindings
```

---

## Use Case 5: Create a Virtual Model Alias (Smart Routing)

Aliases let you define a logical model name that routes to real models, with fallback:

```bash
llmgw --json alias create \
  --name "fast-model" \
  --target "gpt-4o-mini" \
  --fallback "claude-3-haiku"
```

Your application then calls `fast-model` instead of a specific provider model. The gateway handles the routing and fallback automatically.

List all aliases:

```bash
llmgw --json alias list
```

---

## Use Case 6: Revoke and Replace a Compromised Key

```bash
# Revoke immediately
llmgw --json key revoke --id "<key-id>" --yes

# Issue a replacement key
llmgw --json key create --user-id "<user-id>" --name "replacement-key" --budget 50
```

The revoked key stops working immediately (Redis cache is invalidated).

---

## Use Case 7: Add Budget to a Tenant (Top-Up)

```bash
llmgw --json tenant topup --id "<tenant-id>" --amount 100.0 --yes
```

---

## Use Case 8: Inspect Usage Logs

```bash
# Last 50 requests
llmgw --json logs

# Filter by user or key
llmgw --json logs --user-id "<user-id>"

# Usage stats dashboard
llmgw stats
```

---

## Use Case 9: Discover Available Models

Pull the latest model catalog from configured providers:

```bash
llmgw model discover
```

Sync pricing data from LiteLLM:

```bash
llmgw model sync --provider openai
```

---

## Helpful Shorthand

Most `--id` flags accept a name or unique prefix instead of a full UUID:

```bash
# These all work if the name is unique enough
llmgw key revoke --id "alice" --yes
llmgw key revoke --id "alice-dev" --yes
llmgw key revoke --id "550e8400" --yes   # UUID prefix
```

If the name is ambiguous, the CLI will list the matches and ask you to be more specific.
