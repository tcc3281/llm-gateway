# Quick Start: Setting Up a New Project

This guide covers how to provision everything a new project needs to start using the LLM Gateway — tenant, user account, and API key — either step by step or in a single macro command.

---

## Option A: One-Shot Setup (Recommended)

The `env-sync` command handles the entire flow idempotently. Run it from inside the project directory:

```bash
llmgw env-sync --tenant my-app --key-name default --budget 20
```

What it does automatically:
1. Creates the tenant `my-app` (or reuses it if it already exists)
2. Creates a service account user `sa-my-app@system.local`
3. Creates an API key with a $20 budget
4. Writes `OPENAI_API_BASE` and `OPENAI_API_KEY` to the local `.env` file

After running, your `.env` will contain the connection credentials. Your project can now make LLM calls through the gateway.

---

## Option B: Step-by-Step Setup

### Step 1: Create a Tenant

Tenants are top-level billing and access isolation units. Each project or team typically gets one tenant.

```bash
llmgw --json tenant create --name "my-app" --tier "starter"
```

Available tiers: `free`, `starter`, `pro`, `enterprise`

Save the `id` from the response — you'll need it in later steps.

### Step 2: Create a User

Users represent developers or service accounts that will use the gateway.

```bash
llmgw --json user create --email "dev@myteam.com" --name "Dev Account" --role "user"
```

Save the `id` from the response.

### Step 3: Create an API Key

```bash
llmgw --json key create \
  --user-id "user-uuid-here" \
  --name "dev-key" \
  --budget 20 \
  --rpm 60
```

> **Important:** The API key value is only shown once at creation time. Copy it immediately.

The key is used as `OPENAI_API_KEY` in your application:

```bash
export OPENAI_API_BASE="http://localhost:8080/v1"
export OPENAI_API_KEY="sk-..."
```

### Step 4: Verify

Test that the key works with a real model call:

```bash
llmgw chat --model gpt-4o-mini --message "Hello"
```

---

## Check Current Configuration

```bash
llmgw config show
```

This shows the active Gateway URL and confirms whether the admin key is set.
