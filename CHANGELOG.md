# Changelog (Nhật Ký Thay Đổi)

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [v0.4.0] - 2026-09-10

### Added
- **Agentic CLI Bridge & Native MCP Server**:
  - FastMCP stdio server (`llmgw mcp-serve`) integrating natively with AI IDE agents (Cursor, Windsurf, Claude Code, Antigravity) with Human-in-the-Loop (HITL) confirmation for destructive actions.
  - One-command project onboarding macro (`llmgw env-sync --tenant ... --key-name ... --budget ...`) creating tenants, keys, and `.env` idempotently.
  - Granular Model Routing & Policy CLI (`llmgw key model list/add/delete`) supporting regex allowlists and denylists.
  - Destructive action guardrails: enforced `--yes` flag requirement on critical operations (`key revoke`, `tenant delete`, `plugin unbind`).
  - Plugin schema introspection via `llmgw plugin get --name <plugin>`.
  - Comprehensive Agent Skill directory (`skills/llmgw-cli/`) with practical references (`quickstart.md`, `usecases.md`, `error-handling.md`, `mcp-mode.md`) and standard agent rules (`AGENTS.md`, `.cursor/rules/llmgw.mdc`).
  - Automated multi-platform PyInstaller release workflow for Linux and macOS binaries (`.github/workflows/cli-release.yml`).
- **Expanded Core Plugin Catalog (8 New Built-in Plugins)**:
  - `prometheus-metrics`: Native Prometheus exporter for request latency, counter, and token telemetry.
  - `system-prompt-injector`: Dynamic system prompt insertion/overriding scoped by Tenant, Role, or Key.
  - `content-moderation`: Keyword and pattern-based content moderation guardrails.
  - `ip-restriction`: IP CIDR allowlist and denylist enforcement per Tenant, User, or Key.
  - `prompt-compression`: Token optimization by stripping redundant whitespace and repetitive patterns.
  - `response-watermarking`: Dynamic provenance watermarking and custom headers in responses.
  - `canary-traffic-splitter`: Weighted canary traffic splitting across multiple providers and models.
  - `webhook-notifier`: Event-driven webhook alerting on failure thresholds and request lifecycles.
  - Database schema migration `000018_prometheus_and_prompt_injector_plugins` registering new plugins.
- **Client IP Telemetry**:
  - Database schema migration `000019_usage_logs_client_ip` adding `client_ip` column to `usage_logs`.
  - Web Admin UI: Log detail drawer and usage logs table displaying client IP address.

### Changed
- **Architectural Refactoring & Modularization**:
  - Decoupled monolithic PostgreSQL repository (`internal/db/postgres.go`) into focused domain files (`postgres_alerts.go`, `postgres_audit.go`, `postgres_iam_groups.go`, `postgres_logs.go`, `postgres_models_providers.go`, `postgres_plugins.go`, `postgres_tenants_billing.go`, `postgres_users_keys.go`, `postgres_virtual_models.go`).
  - Decoupled monolithic Gateway service (`internal/service/gateway.go`) into modular domain components (`gateway_discovery.go`, `gateway_embeddings.go`, `gateway_routing.go`, `gateway_stream.go`, `gateway_telemetry.go`).
- **Web Admin UI Improvements**:
  - Extended `PluginBindingModal` with specialized configuration forms for newly added plugins.
  - Added CLI Quickstart card to Settings page.
  - Bumped UI Pipeline version badge to `v0.4.0`.
- **Version Synchronization**:
  - Synchronized version references across Web Admin UI (`web`), CLI tool (`llmgw-cli`), Security Policy, and Roadmap to `v0.4.0`.

---

## [v0.3.1] - 2026-09-09

### Fixed
- Fixed missing `model` package import in `internal/plugin/builtin/builtin_test.go` causing `go vet` failure during GitHub Actions CI.

### Changed
- Synchronized version references across Web Admin UI (`web`), CLI tool (`llmgw-cli`), and Security Documentation to `v0.3.1`.

---

## [v0.3.0] - 2026-09-09

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
