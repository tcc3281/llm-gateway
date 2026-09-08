# 📢 Community Showcase & Launch Templates

This document contains pre-written, authentic, and developer-friendly launch posts formatted for **Hacker News (Show HN)** and **Reddit** (`r/LocalLLaMA`, `r/golang`, `r/selfhosted`).

---

## 1. Hacker News (Show HN)

### Title:
```text
Show HN: LLM Gateway – An open-source distributed AI gateway in Go with AWS-style IAM
```

### First Comment / Post Body:
```markdown
Hi HN,

I built LLM Gateway (https://github.com/tcc3281/llm-gateway) because we noticed a common problem when deploying LLM proxies in production:

Traffic characteristics of AI streaming (holding thousands of concurrent HTTP/SSE connections, low memory footprint, networking throughput) are completely opposite to management traffic (complex relational queries, RBAC evaluation, auditing, financial billing). When everything is crammed into a single monolithic Python proxy, an expensive analytics query or a spike in admin requests can choke the event loop, causing client SSE streams to drop or jitter.

LLM Gateway addresses this with a distributed, microservices-friendly architecture written in Go:

1. **Separation of Planes**:
   - `proxy` (port 8080): High-performance data plane for `/v1/chat/completions` and `/v1/messages`. It translates payloads across OpenAI, Anthropic, and Gemini transparently, streaming tokens back with sub-millisecond Redis key authentication.
   - `control-plane` (port 8081): Houses the ACID management state (users, organizations, model pricing, policy definitions) and serves an embedded React Admin SPA.
   - `worker`: Asynchronously evaluates sliding-window anomalies (latency spikes, budget breaches, error rate surges) and dispatches webhooks to Slack, Discord, PagerDuty, or HMAC-signed endpoints.
   - For simple or local setups, everything can still run in a single container (`all-in-one`).

2. **Enterprise IAM**: Instead of basic role dropdowns, it implements AWS IAM-style statement policies (`Effect: Allow/Deny`, action namespaces like `model:use`, `analytics:read`, model wildcards like `model:claude-*`), with explicit deny always winning.

3. **Multi-Tenant B2B Accounting**: Native prepaid wallet debits and postpaid monthly ceilings backed by an immutable ledger.

4. **Zero-Leak Modality Telemetry**: We inspect and display the structural shape of requests (whether a prompt included text, vision images, audio, or tool calls) without logging sensitive plaintext prompts to disk.

The project is MIT-licensed, includes Docker Compose for a 60-second quickstart, and has a native bilingual UI (English + Vietnamese).

Repo: https://github.com/tcc3281/llm-gateway

Would love to hear your thoughts, feedback, and architectural critiques!
```

---

## 2. Reddit (`r/LocalLLaMA` / `r/selfhosted`)

### Title:
```text
I built an open-source distributed LLM Gateway in Go with AWS IAM-style policies and multi-tenant billing
```

### Post Content:
```markdown
Hey everyone,

While setting up infrastructure for teams consuming various LLM providers (OpenAI, Claude, Gemini, and local vLLM/Ollama endpoints), we kept hitting limitations with traditional monolithic Python proxies:
- A heavy audit log query in the admin dashboard would cause active SSE streams to lag or drop.
- Access control was usually just "Admin" vs "User", with no easy way to restrict expensive models (e.g. Claude 3.5 Sonnet allowed, Opus denied) per team or department.
- No clean way to handle B2B prepaid credits or send alerts when an unexpected budget surge occurs.

To solve this, I built **LLM Gateway** (Go + Docker):
👉 **GitHub**: https://github.com/tcc3281/llm-gateway

### What makes it different:
1. **Universal Protocol Translation**: Call any model through standard `/v1/chat/completions` (OpenAI format) or `/v1/messages` (Anthropic format). It also supports transparent passthrough of extended reasoning/thinking (`reasoning_effort` and `thinking` budget tokens).
2. **AWS IAM-style JSON Policies**: Visual policy editor to define fine-grained statements: Allow/Deny, action namespaces (`model:*`, `analytics:*`, `key:*`), and model wildcards (`model:gpt-*`).
3. **Multi-Tenant Accounting**: Create internal teams or external B2B clients with prepaid credit balances or monthly postpaid ceilings + transaction ledger.
4. **Proactive Multi-Channel Alerting**: Evaluates budget thresholds, error rate surges, and latency spikes across sliding windows, dispatching to Slack, Discord, PagerDuty, or generic webhooks with HMAC-SHA256 signatures.
5. **Zero-Leak Inspection Drawer**: Deep inspection of request modality (vision, audio, tool calling, token composition, TTFT timeline) without storing sensitive plaintext.
6. **Embedded Admin UI**: Dark-mode Obsidian Telemetry dashboard served directly from the Go binary with instant English / Vietnamese localization.

### Quickstart (Docker):
```bash
git clone https://github.com/tcc3281/llm-gateway.git
cd llm-gateway
docker compose up -d
```
Public Gateway runs on `:8080`, and Admin Web UI runs on `:8081`.

It is 100% open-source under the MIT license. I’d love to get your feedback and see what features or local model adapters you'd like to see next!
```

---

## 3. GitHub Topics to Add to Repository Settings:

Go to your GitHub repo $\rightarrow$ **About** (gear icon on the top right) $\rightarrow$ Add these topics:
```text
llm, ai-gateway, llm-gateway, openai, anthropic, gemini, reverse-proxy, golang, docker, iam, multi-tenant, alerting, developer-tools, observability
```
