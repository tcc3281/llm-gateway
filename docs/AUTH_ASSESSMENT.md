# LLM Gateway - Auth & Access Control Assessment

> Ngày review: 2026-09-08
> Trạng thái hiện tại: MVP / Prototype
> Mục tiêu: Production-ready multi-tenant with IAM-style permissions

---

## 1. Tổng quan kiến trúc auth hiện tại

```
Client → Bearer token / x-api-key header
       → Gateway Handler.authenticate()
         → ValidateAPIKey (Redis cache → PostgreSQL fallback)
           → Check rate limit (RPM per key)
           → Check budget quota (lifetime USD per key)
         → Resolve upstream provider (catalog-driven routing)
         → Proxy to upstream
```

**Admin Portal**: IP whitelist + `X-Admin-API-Key` header (port 8081)

---

## 2. Vấn đề phát hiện

### 🔴 CRITICAL

| # | Vấn đề | File | Dòng |
|---|--------|------|------|
| C1 | User role (`role` field) tồn tại nhưng **zero enforcement** - bất kỳ key nào cũng gọi được mọi model | `handler/gateway.go` | authenticate() |
| C2 | Không có model restriction per key/user - cho key = cho tất | `handler/gateway.go` | authenticate() |
| C3 | Timing attack trên admin key compare (`==` operator) | `auth/auth.go` | 101-103 |
| C4 | Default admin key `sk-admin-secret-key-change-me` nếu forget env | `config/config.go` | 46 |
| C5 | Budget race condition: 100 concurrent requests vượt budget limit | `handler/gateway.go` | 275-301 |
| C6 | Không có audit log cho admin actions | `handler/admin.go` | toàn bộ |

### 🟡 SHOULD FIX

| # | Vấn đề | File | Dòng |
|---|--------|------|------|
| S1 | IP whitelist bypass qua X-Forwarded-For spoofing | `handler/admin.go` | 63-76 |
| S2 | Admin key static trong env, không rotate được | `config/config.go` | 46 |
| S3 | Budget chỉ per-key, không có user-level aggregation | `handler/gateway.go` | 275 |
| S4 | Budget lifetime, không có period (daily/monthly) | `model/models.go` | 24 |
| S5 | Không có per-key model allowlist/denylist | - | - |
| S6 | Không có group/team concept | - | - |

### 🟢 ĐÃ LÀM TỐT

- SHA-256 hashing API keys (không lưu plaintext)
- Redis caching cho hot-path auth (sub-ms)
- Key lifecycle: create/revoke/reactivate + immediate cache invalidation
- AES-256-GCM encryption cho provider keys at rest
- Rate limiting per-key RPM sliding window
- Key expiry support (`expires_at`)
- Fallback chain với exponential backoff

---

## 3. Đánh giá mức độ

| Khía cạnh | Điểm | Ghi chú |
|-----------|------|---------|
| API Key Auth | 7/10 | Solid foundation, timing attack gap |
| Role/RBAC | 1/10 | Field tồn tại, zero enforcement |
| Model Restriction | 0/10 | Không có - biggest gap |
| Budget Control | 5/10 | Per-key lifetime, race condition |
| Rate Limiting | 6/10 | Working but no per-user/per-model |
| Group/Team | 0/10 | Không có |
| Admin Security | 4/10 | IP whitelist good, static key + no audit |
| Encryption at Rest | 8/10 | AES-256-GCM |
| Audit Trail | 1/10 | Telemetry có, admin audit không |

**Overall: ~35% production-ready** cho multi-tenant enterprise.

---

## 4. Schema hiện tại (cần mở rộng)

```sql
-- Hiện có
users (id, email, name, role, created_at, updated_at)
api_keys (id, user_id, key_prefix, key_hash, name, budget_limit, rate_limit_rpm, is_active, expires_at, created_at)

-- Cần thêm cho IAM-style
groups (id, name, description, created_at)
group_policies (id, group_id, policy_json, created_at)
user_groups (user_id, group_id, created_at)
key_model_policies (id, key_id, model_pattern, is_allowlist)
audit_logs (id, actor_id, actor_type, action, target_type, target_id, details, ip_address, created_at)
budget_resets (id, key_id, period, last_reset_at)
```

---

## 5. Referenced by

- `docs/AUTH_PLAN.md` - Implementation plan cho IAM-style permission system
