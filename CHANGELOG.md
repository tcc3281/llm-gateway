# Changelog (Nhật Ký Thay Đổi)

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [v0.3.0] - Unreleased (Staging)

### Added
- **Kong-like Multi-Level Plugin Architecture**:
  - Pluggable request lifecycle pipeline with 4 interception hooks: `PreAuth`, `PostAuth`, `PreRouting`, and `PostUpstream`.
  - 7-Level Scope Hierarchy Precedence: `API Key > User > Role > Tenant > Model > Provider > Global`.
  - Global and instance-level Plugin Engine registry with runtime binding evaluation and short-circuit capability.
  - Database schema migration `000017_plugin_architecture` with `plugins` catalog and `plugin_bindings` tables.
- **10 First-Party Modular Core Plugins**:
  - `rate-limiting`: Redis token-bucket throughput rate limiting (RPM/TPM/Burst).
  - `budget-control`: Enforces USD spend budgets with automatic blocking (402) or warning.
  - `pii-masking`: Sanitizes sensitive personal information (Email, Phone, Credit Card, National ID/CCCD, API Keys/Secrets) before forwarding prompts.
  - `prompt-guard`: Dedicated security guardrail detecting and blocking prompt injection, DAN jailbreaks, system prompt extraction, and toxic banned keywords.
  - `semantic-cache`: Sub-10ms response caching in Redis for non-streaming requests with `X-Cache: HIT/MISS` headers.
  - `request-hedging`: Dual-dispatch configuration hints for VIP low-latency queries.
  - `header-transform`: Injects, mutates, or removes custom HTTP headers sent to upstream AI providers.
  - `retry-circuit-breaker`: Fast-failing circuit breaker tripping when upstream error thresholds are exceeded.
  - `langfuse-exporter`: Asynchronous non-blocking export of request traces, token usages, and latencies to Langfuse Ingestion API.
  - `audit-trail`: Structured immutable security audit logs and webhook forwarding.
- **Virtual Models & Dynamic Routing**:
  - Decouples client code from real providers with business aliases (e.g. `fast-chat`, `reasoning-pro`).
  - Weighted Canary traffic splitting across providers and models.
  - Automatic fallback ladders and circuit routing on upstream failures.
- **Obsidian Telemetry Bento Grid Admin UI**:
  - Redesigned Plugins administration page at `/plugins` with dark mission-control theme.
  - 4 KPI telemetry cards, category filter tabs, scope dropdown, status filter, and live scope inheritance tree preview.
  - Visual configuration builder modal (`PluginBindingModal`) with dedicated form fields for each of the 10 plugins + Raw JSON Editor toggle.

### Changed
- Refactored `GatewayService` request pipeline to invoke plugin hooks seamlessly around authentication, IAM evaluation, and upstream calls.
- Updated Gemini and OpenAI adapter streaming protocols to support thought-signature roundtrips and OpenCode SSE `finish_reason` mappings.

---

## [v0.2.2] - 2026-09-08

### Added
- Comprehensive future technical roadmap (`docs/ROADMAP.md`) detailing Plugin Architecture, Cell-Based Isolation, and Cluster-Native Mesh Scaling.
- Internationalized README in Vietnamese (`README.vi.md`) and English (`README.md`).
- Community showcase guide (`docs/SHOWCASE_POSTS.md`).
- Standard GitHub issue templates (`bug_report.yml`, `feature_request.yml`) and pull request template.

---

## [v0.2.1] - 2026-09-08

### Added
- Official open-source MIT License (`LICENSE`).
- Comprehensive Contributing Guidelines (`CONTRIBUTING.md`).
- Automated Gitleaks secret scanning in CI workflow (`.gitleaks.toml`).

### Changed
- Streamlined project Makefile targets for building, testing, linting, and Docker operations.

---

## [v0.2.0] - 2026-09-08

### Added
- **Proactive Alerting & Webhooks Engine**:
  - Multi-config notification channels supporting Slack, Discord, Telegram, and generic Webhooks.
  - Metric alerting rules: high error rate surges, cost spikes, and IAM policy denial surges.
  - Administration UI for configuring alert policies, thresholds, and evaluating trigger histories.

---

## [v0.1.0] - 2026-09-08

### Added
- **Enterprise Foundation & Distributed Architecture**:
  - Microservices layout: `gateway` proxy, `control-plane` admin API, and `worker` asynchronous aggregator.
  - AWS IAM-style JSON access policies for role-based and granular model permission control.
  - Multi-tenant B2B department management with credit balance ledger and monthly budgets.
  - Real-time token consumption tracking, cost estimation, and audit logging.
  - Web Admin Dashboard with analytics charts, usage logs viewer, and API key management.
