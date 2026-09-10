# Error Handling & Troubleshooting

When `llmgw --json` fails, it always returns a structured error:

```json
{
  "status": "error",
  "code": "HTTP_401",
  "message": "Unauthorized",
  "suggested_action": "Check that the admin API key is correct."
}
```

Use `code` to decide what to do next. Below are the most common errors and their fixes.

---

## Auth Errors

### `HTTP_401` — Unauthorized

The admin API key is wrong or not set.

```bash
# Check what key is currently configured
llmgw config show

# Reconfigure with the correct key
llmgw config set --url http://localhost:8081 --key sk-admin-secret
```

The correct admin key comes from the server's environment: `ADMIN_API_KEY` in `.env` or docker-compose.

### `HTTP_403` — Forbidden

Your IP is not on the gateway's allowlist (`ADMIN_ALLOWED_IPS`). This is a server-side setting — ask the gateway administrator to allow your IP, or connect via VPN/internal network.

---

## Resource Errors

### `HTTP_404` — Not Found

The resource ID doesn't exist.

```bash
# Get a fresh list to find the correct ID
llmgw --json tenant list
llmgw --json key list
llmgw --json user list
```

Check for typos. Note that name/prefix matching is case-sensitive.

### `AMBIGUOUS_MATCH` — Multiple matches

The name or prefix you gave matches more than one resource.

```bash
# The error output lists all matching items:
# { "code": "AMBIGUOUS_MATCH", "message": "Multiple matches found" }

# Fix: use the full UUID instead
llmgw --json key list   # find the exact UUID
llmgw key revoke --id "550e8400-e29b-41d4-a716-446655440000" --yes
```

### `NOT_FOUND` — No match

No resource matched your name/prefix. Check spelling, or list resources to see what's available.

---

## Validation Errors

### `HTTP_400` — Bad Request

Usually a field validation error. The `message` field tells you what's wrong:

- `"name already exists"` → That name is taken. Use `list` to find the existing resource and reuse its ID.
- `"invalid tier"` → Valid tenant tiers are: `free`, `starter`, `pro`, `enterprise`.
- `"invalid JSON config"` → Plugin config must be valid JSON. Check for unescaped quotes.

### `CONFIRMATION_REQUIRED`

You ran a destructive command without `--yes` in a non-interactive context:

```bash
# Add --yes to proceed
llmgw --json key revoke --id "my-key" --yes
```

---

## Connection Errors

### `connection_error: ...` — Cannot reach gateway

The gateway is not running or the URL is wrong.

```bash
# Check that the gateway is up
llmgw health

# If health fails, check the URL
llmgw config show

# Reconfigure if needed
llmgw config set --url http://correct-host:8081 --key sk-admin-secret
```

Common causes:
- Gateway container is stopped → `docker compose up -d`
- Wrong port — the admin API is on port `8081` (not `8080`, which is the public LLM proxy)
- Firewall blocking the port

---

## Rate / Budget Errors (LLM Calls, Not CLI)

These appear when making LLM calls through the gateway, not the CLI itself.

### `HTTP_429` — Rate limit exceeded

The key's RPM limit was hit. Either wait and retry, or increase the key's rate limit:

```bash
llmgw --json key update --id "<key-id>" --rpm 200
```

### `HTTP_402` / Budget exhausted

The key or tenant has spent its budget. Top up the tenant:

```bash
llmgw --json tenant topup --id "<tenant-id>" --amount 50.0 --yes
```

Or increase the key's budget limit:

```bash
llmgw --json key update --id "<key-id>" --budget 100
```

### `HTTP_403` — Model not allowed

The key has a model policy that blocks the requested model. Check the rules:

```bash
llmgw --json key model list --key-id "<key-id>"
```

Remove the blocking rule or add an allowlist rule for the model you need.

---

## Plugin / Guardrail Blocks

If requests are being rejected by a plugin (e.g., prompt injection detected), check active bindings:

```bash
llmgw --json plugin bindings
```

To temporarily disable a binding:

```bash
llmgw --json plugin unbind --id "<binding-id>" --yes
```

To view the schema and understand config options:

```bash
llmgw --json plugin get --name "prompt-guard"
```

---

## General Debugging Checklist

1. Run `llmgw health` — is the gateway reachable?
2. Run `llmgw config show` — is the URL and key set correctly?
3. Run `llmgw --json <your-command>` — read the `code` and `message` fields carefully.
4. Run `llmgw logs` to see recent request history and spot failures.
5. Check the gateway server logs via `docker compose logs gateway`.
