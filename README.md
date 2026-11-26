# LLM Gateway

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://golang.org)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?logo=docker)](https://www.docker.com)
[![GCP Cloud Run](https://img.shields.io/badge/Deploy-GCP%20Cloud%20Run-4285F4?logo=googlecloud)](https://cloud.google.com/run)
[![Terraform](https://img.shields.io/badge/IaC-Terraform-7B42BC?logo=terraform)](https://www.terraform.io)
[![i18n: vi/en](https://img.shields.io/badge/i18n-VI%20%7C%20EN-blue)](web/src/i18n)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)

> 🌐 **Language:** **English** | [Tiếng Việt](README.vi.md)  
> 🚀 **Live Production Demo:** **[https://vin-ai-t205.io.vn](https://vin-ai-t205.io.vn)**  
> 🔑 **Recruiter / Demo 1-Click Login:** Click the **`🚀 Đăng nhập nhanh (Nhà tuyển dụng / Demo)`** button on the login screen (pre-fills `admin@llm-gateway.internal` / `admin`).

An enterprise-grade, high-performance distributed LLM Gateway written in **Go**. Features unified OpenAI & Anthropic protocol adaptation, extensible Guardrail Plugin Engine (Prompt Injection, PII Masking, Semantic Caching), AWS IAM-style statement policies, dynamic upstream model discovery & LiteLLM pricing catalog sync, multi-tenant B2B accounting with prepaid credit ledgers, proactive multi-channel alerting (Slack, Discord, PagerDuty, HMAC Webhooks), and zero-leak multimodal telemetry.

---

## ⚡ Live Demo & Quick Test

You can test the live gateway running on Google Cloud Run right away:

```bash
# 1. Test unified OpenAI-compatible Chat Completion via the Gateway
curl -s -X POST https://vin-ai-t205.io.vn/v1/chat/completions \
  -H "Authorization: Bearer <YOUR_API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "messages": [{"role": "user", "content": "Explain VinAI LLM Gateway in one sentence!"}]
  }'
```

👉 **Access Admin Web UI**: Open **[https://vin-ai-t205.io.vn](https://vin-ai-t205.io.vn)** in your browser.  
👉 **1-Click Recruiter Login**: Use the quick demo button on the login screen to explore the full dashboard, IAM policies, tenants, and telemetry.

---

## ⚡ 60-Second Local Quickstart

Get the entire stack (Gateway, Admin Web UI, PostgreSQL, Redis) running locally in under a minute with Docker:

```bash
# 1. Clone repository
git clone https://github.com/tcc3281/llm-gateway.git
cd llm-gateway

# 2. Launch with Docker Compose
docker compose up -d

# 3. Test a unified chat completion through local gateway (Port 8080)
curl -s http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-gw-8f7a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Explain LLM Gateway in one sentence!"}]
  }'
```

👉 **Local Admin Web UI**: Open [http://localhost:8081](http://localhost:8081) (or port 8080).  
🔑 **Default Admin Key**: `sk-admin-secret-key-change-me` (configured in `docker-compose.yml`).

---

## 🎯 Why LLM Gateway? (Key Differentiators)

Most open-source LLM proxies are built as monolithic Python applications where AI stream forwarding and heavy administrative operations share the same thread pool and database connections. **LLM Gateway** takes a distributed, enterprise-first approach:

| Capability | LLM Gateway | Traditional Monolithic Proxies |
| :--- | :--- | :--- |
| **Architecture** | **Distributed microservices** (`proxy`, `control-plane`, `worker`) or lightweight `all-in-one` | Monolith (management queries can block SSE streaming) |
| **Performance** | **High-concurrency Go engine** with Redis Token Bucket & sub-millisecond key validation | Often Python GIL-bound under thousands of concurrent streams |
| **Access Control** | **AWS IAM-style JSON policies** (`Effect: Allow/Deny`, actions namespace, wildcards, explicit deny precedence) | Basic role dropdowns (Admin/User) |
| **Multi-Tenancy** | **B2B Tenants** with prepaid credit balance, monthly ceilings, and immutable financial ledger | Simple tag-based grouping without credit ledgers |
| **Alerting** | **Proactive sliding-window alerting** to Slack, Discord, PagerDuty, and generic webhooks with HMAC-SHA256 | Static notifications or external alertmanager dependency |
| **Telemetry & Privacy** | **Zero-leak structural shape tracing** (modality detection for vision/audio/tools without recording sensitive plaintext) | Plaintext storage or all-or-nothing redaction |
| **Internationalization** | **100% Native Bilingual Web UI** (Vietnamese 🇻🇳 & English 🇬🇧) with instant zero-reload toggle | English only |

---

## 🏗️ Architecture

LLM Gateway cleanly separates high-throughput AI traffic from control-plane administrative traffic:

```
                          ┌────────────────────────┐
                          │  Client Applications   │
                          │  (OpenAI / Anthropic)  │
                          └───────────┬────────────┘
                                      │
               ┌──────────────────────┴──────────────────────┐
               │                                             │
      [Port 8080: AI Traffic]                       [Port 8081: Admin]
               │                                             │
               ▼                                             ▼
     ┌───────────────────┐                         ┌───────────────────┐
     │   Gateway Proxy   │                         │   Control Plane   │
     │   (Data Plane)    │                         │  (Management UI)  │
     └─────────┬─────────┘                         └─────────┬─────────┘
               │                                             │
               ├──────────────────────┬──────────────────────┤
               ▼                      ▼                      ▼
        ┌─────────────┐        ┌─────────────┐        ┌─────────────┐
        │ Redis Cache │        │ PostgreSQL  │        │   Worker    │
        │  & Limiter  │        │ (ACID Data) │        │ (Alerting)  │
        └─────────────┘        └─────────────┘        └──────┬──────┘
                                                             │
                                                             ▼
                                                    [Slack / Webhooks]
```

### Tri-Service Split:
1. **`proxy` (Port 8080)**: High-performance data plane for client requests (`/v1/chat/completions`, `/v1/messages`). Dispatches to upstream providers (OpenAI, Anthropic, Gemini), translates SSE real-time, calculates cost, and buffers usage events.
2. **`control-plane` (Port 8081)**: Manages users, groups, tenants, IAM policies, model catalog, and serves the embedded Admin SPA.
3. **`worker`**: Background asynchronous consumer that ingests usage metrics, evaluates sliding-window alert thresholds, and dispatches multi-channel webhooks.

---

## ✨ Features

### 1. Universal Protocol Adapter (Inbound / Outbound)
- **Call any model via OpenAI format (`/v1/chat/completions`)**: Use OpenAI SDKs to invoke Claude 3.5 Sonnet, Gemini 1.5 Pro, or GPT-4o with zero client code modification.
- **Call any model via Anthropic format (`/v1/messages`)**: Use native Anthropic SDKs seamlessly.
- **Cross-Provider Extended Thinking**: Transparent passthrough of OpenAI `reasoning_effort` (low/medium/high) and Anthropic `thinking` budget tokens.

### 2. AWS IAM-Style Fine-Grained Policies
- Full visual policy editor & raw JSON editor in the Admin UI.
- Supports explicit allow/deny, action namespaces (`model:*`, `analytics:*`, `key:*`, `user:*`), model wildcards (`model:gpt-*`, `model:claude-*`), and explicit deny precedence.
- Key-level model allowlists and denylists for granular credential sandboxing.

### 3. Multi-Tenant B2B Accounting
- Create **Internal Departments** or **External B2B Clients**.
- **Prepaid Mode**: Automatically debits tenant credit balance per request and blocks traffic when depleted.
- **Postpaid Mode**: Enforces monthly spending ceilings.
- **Credit Audit Ledger**: Full history of deposits, wire contract references, and auditor notes.

### 4. Proactive Multi-Channel Alerting
- Real-time evaluation across 5 critical event types:
  - `budget_threshold`: Spending limit alerts.
  - `error_rate_surge`: Upstream provider failures.
  - `latency_anomaly`: Average response time spikes.
  - `rate_limit_surge`: RPM threshold breaches.
  - `iam_deny_surge`: Security unauthorized access spikes.
- Multi-destination dispatching: **Generic Webhook** (with HMAC-SHA256 signature verification), **Slack**, **Discord**, and **PagerDuty**.
- Sliding time windows (e.g. evaluate over 5 minutes) and cooldown deduplication to eliminate alert fatigue.

### 5. Zero-Leak Modality Telemetry & Audit
- Deep inspection drawer for every request:
  - **Telemetry Timeline**: TTFT (Time-to-first-token), total generation duration, and stream status.
  - **Token Breakdown**: Prompt, completion, reasoning tokens, and cache savings percentage.
  - **Structural Modality Shape**: Inspects whether the request contained text, vision images, audio, or tool calls without storing plaintext sensitive customer data.
  - **Reproducible Export**: One-click copy as curl command or JSON request payload.

### 6. Obsidian Telemetry Web UI
- Built with React 18, Vite, and TypeScript.
- Embedded directly into the Go binary (`go:embed`) — zero separate frontend deployments required.
- **100% Native Internationalization**: Instant toggle between 🇻🇳 Tiếng Việt and 🇬🇧 English, persisted in local storage.

---

## 🚀 Deployment

### Option A: Standard All-in-One (Recommended for Quickstart)
Runs everything in a unified container stack:
```bash
docker compose up -d
```

### Option B: Distributed Microservices (Enterprise Scale)
Runs separate containers for `proxy`, `control-plane`, and `worker` for independent horizontal scaling:
```bash
docker compose -f docker-compose.distributed.yml up -d --build
```

---

## 🛠️ Project Structure

```text
.
├── cmd/
│   ├── all-in-one/               # Unified single-binary entrypoint
│   ├── control-plane/            # IAM, Admin API & Web UI microservice
│   ├── gateway/                  # Gateway entrypoint
│   ├── proxy/                    # High-throughput AI proxy microservice
│   └── worker/                   # Async alerting & log aggregation worker
├── cli/                          # llmgw Python CLI management tool (uv)
├── internal/
│   ├── adapter/                  # Protocol translations (OpenAI, Anthropic, Gemini)
│   ├── alert/                    # Alerting engine & multi-channel webhook dispatchers
│   ├── analytics/                # Token counter, cost calculator, async logger
│   ├── auth/                     # API Key validation & hashing
│   ├── config/                   # Environment configuration loader
│   ├── db/                       # PostgreSQL & Redis connection pools
│   ├── handler/                  # HTTP route handlers (Gateway, Admin, IAM, Alerts)
│   ├── iam/                      # Unified IAM orchestration service
│   ├── model/                    # Domain data structs
│   ├── permission/               # AWS IAM statement policy evaluation engine
│   ├── provider/                 # Upstream HTTP transport client
│   ├── service/                  # Core gateway orchestration logic
│   └── telemetry/                # Modality inspection & zero-leak structural shape tracing
├── migrations/                   # SQL migration scripts (000001 -> 000015)
├── pkg/sse/                      # High-performance Server-Sent Events parser
├── web/                          # Admin Web UI (React 18 + Vite + TypeScript + i18n)
├── Dockerfile                    # Multi-stage optimized Docker build
├── docker-compose.yml            # All-in-One Docker Compose stack
├── docker-compose.distributed.yml# Distributed microservices Docker Compose stack
├── CONTRIBUTING.md               # Open source contribution guidelines
├── LICENSE                       # MIT License
└── README.md                     # English documentation
```

---

## 🤝 Contributing

Contributions are warmly welcomed! Whether you are reporting a bug, proposing a new feature, improving documentation, or adding a provider adapter:

1. Read the [CONTRIBUTING.md](CONTRIBUTING.md) guide.
2. Check existing [Issues](https://github.com/tcc3281/llm-gateway/issues) or open a new one.
3. Submit a Pull Request following our conventional commit standards.

---

## 📄 License

This project is open-sourced under the **[MIT License](LICENSE)**. You are free to use, modify, distribute, and commercialize it.
