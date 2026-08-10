# 🛠 LLM Gateway — Cheat Sheet

Mọi lệnh và endpoint quan trọng của **LLM Gateway** (Go + Docker), gồm Admin CLI (`llmgw`), Public API và Admin API.

---

## 1. Kiến trúc 2 Port

| Port  | Vai trò            | Routes                                  | Auth                        |
|-------|--------------------|-----------------------------------------|-----------------------------|
| 8080  | Public LLM Gateway | `/v1/chat/completions`, `/v1/messages`, `/v1/models`, `/health` | `Authorization: Bearer sk-gw-...` hoặc `x-api-key` |
| 8081  | Admin Management   | `/admin/...`                             | Header `X-Admin-API-Key`    |

---

## 2. Khởi động

```bash
cp .env.example .env
# Sửa .env: ADMIN_API_KEY, và (tuỳ chọn) PROVIDER_MASTER_KEY

docker-compose up -d --build

# Healthcheck
curl http://localhost:8081/health
```

`.env` **chỉ chứa cấu hình hệ thống**. Key của OpenAI/Anthropic/Gemini được lưu trong DB và cấu hình bằng CLI/Admin API (xem mục 4).

---

## 3. Env Variables

| Biến | Mô tả | Default |
|------|-------|---------|
| `PORT` | Cổng public gateway | `8080` |
| `ADMIN_PORT` | Cổng admin | `8081` |
| `ENV` | Môi trường | `development` |
| `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASSWORD` / `DB_NAME` / `DB_SSLMODE` | Postgres | `postgres` / `5432` / `llm_user` / `llm_password` / `llm_gateway` / `disable` |
| `REDIS_HOST` / `REDIS_PASSWORD` | Redis | `redis:6379` / empty |
| `ADMIN_API_KEY` | Secret bảo vệ Admin API | `sk-admin-secret-key-change-me` |
| `PROVIDER_MASTER_KEY` | Mã hoá provider keys trong DB (AES-256-GCM). Trống = plaintext (dev) | empty |

---

## 4. Admin CLI (`llmgw`)

Chạy trong container: `docker exec -it llm_gateway llmgw ...`

### Users
```bash
llmgw user create --email dev@company.com --name "Developer 1" --role user
llmgw user list
```

### API Keys
```bash
llmgw key create --user-id <USER_UUID> --name "Production App Key" --budget 50.0 --rpm 100
llmgw key list
llmgw key revoke --id <KEY_UUID>        # vô hiệu hoá ngay (cả cache Redis)
```

### Providers (stateful keys trong DB)
```bash
llmgw provider set --provider openai     --api-key sk-proj-...
llmgw provider set --provider anthropic  --api-key sk-ant-...
llmgw provider set --provider gemini     --api-key AIzaSy...
llmgw provider list                      # secret được che giấu
llmgw provider delete --provider openai
```

### Models & Pricing
```bash
llmgw model list
llmgw model add --provider openai --name gpt-4o --prompt-price 2.50 --completion-price 10.00
llmgw model delete --id <MODEL_UUID>
```

### Stats
```bash
llmgw stats --interval hour             # hoặc day
llmgw stats --interval day --user-id <USER_UUID>
```

---

## 5. Public API (Port 8080)

### Liệt kê models (bắt buộc auth → 401 nếu thiếu key; lấy thật từ provider, cache Redis 1h)
```bash
curl http://localhost:8080/v1/models \
  -H "Authorization: Bearer sk-gw-..."
```

### Chat completions — OpenAI format
```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-gw-8f7a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a" \
  -d '{"model": "gpt-4o", "messages": [{"role": "user", "content": "Xin chào!"}]}'
```

Gọi model Anthropic qua chuẩn OpenAI (auto map):
```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-gw-..." -H "Content-Type: application/json" \
  -d '{"model": "claude-3-5-sonnet-20240620", "messages": [{"role": "user", "content": "Hi"}]}'
```

### Messages — Anthropic format
```bash
curl http://localhost:8080/v1/messages \
  -H "x-api-key: sk-gw-..." -H "Content-Type: application/json" \
  -d '{"model": "claude-3-5-sonnet-20240620", "max_tokens": 1024, "messages": [{"role": "user", "content": "Hi"}]}'
```

### Streaming
Thêm `"stream": true` vào payload. Gateway trả SSE theo **định dạng của inbound**:
- OpenAI inbound → `data: {"choices":[{"delta":{"content":"..."}}]}` + `data: [DONE]`
- Anthropic inbound → `message_start`, `content_block_start`, `content_block_delta`, `message_delta`, `message_stop`

### Upstream key override (per-request)
Client có thể gửi key riêng của mình cho 1 request:
```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-gw-..." -H "x-upstream-api-key: sk-proj-client-own-key" \
  -d '{"model": "gpt-4o", "messages": [{"role": "user", "content": "Hi"}]}'
```

---

## 6. Admin API (Port 8081)

Luôn kèm header: `X-Admin-API-Key: sk-admin-secret-key-change-me`

| Method | Path | Mô tả |
|--------|------|-------|
| POST | `/admin/users` | Tạo user |
| GET | `/admin/users` | List users |
| POST | `/admin/keys` | Tạo API key (`api_key` trả về 1 lần duy nhất) |
| GET | `/admin/keys` | List API keys |
| DELETE | `/admin/keys?id=<uuid>` | Revoke key |
| POST | `/admin/providers` | Upsert provider key `{"provider","api_key"}` |
| GET | `/admin/providers` | List providers (che secret) |
| DELETE | `/admin/providers?provider=openai` | Xoá provider key |
| GET | `/admin/models` | List model pricing |
| POST | `/admin/models` | Upsert model pricing |
| DELETE | `/admin/models?id=<uuid>` | Xoá model |
| GET | `/admin/analytics/stats?interval=hour` | Usage stats (hỗ trợ `user_id`, `from`, `to`) |

*Mọi path `/admin/*` đều có alias không prefix (`/users`, `/keys`, `/providers`, `/models`, `/analytics/stats`).*

---

## 7. Auth, Rate Limit & Quota

- **Auth API key**: hash SHA-256 của `sk-gw-...`, cache Redis `apikey:<hash>` (TTL 10 phút) → validate < 1ms.
- **Rate limit (RPM)**: Redis token bucket `ratelimit:<keyID>:<minute>`. Vượt → `429 Rate limit exceeded`.
- **Budget quota**: tổng `cost` trong `usage_logs` so với `budget_limit` → vượt → `429 Insufficient quota`.
- **Lỗi thiếu provider key**: `503` kèm code `provider_key_missing` (khác với lỗi upstream).

---

## 8. Database (PostgreSQL)

| Bảng | Mục đích |
|------|----------|
| `users` | Người dùng (email, name, role) |
| `api_keys` | Gateway API keys (hash, budget, rpm, active) |
| `model_pricings` | Giá per 1M token của từng model |
| `provider_keys` | **Provider credentials (stateful, có thể mã hoá)** |
| `usage_logs` | Telemetry mỗi request (token, cost, TTFT, latency) |

Migrations chạy tự động lúc khởi động (`migrations/*.up.sql` theo thứ tự tên file).

**Cost model chưa có?** → gateway tự fallback: `$1/1M prompt + $3/1M completion`.

---

## 9. Xử lý lỗi nhanh

| Triệu chứng | Nguyên nhân | Xử lý |
|-------------|-------------|-------|
| `503 provider_key_missing` | Chưa set key provider | `llmgw provider set --provider openai --api-key ...` |
| `401 invalid_api_key` | Sai/thiếu `sk-gw-...` | Tạo key mới: `llmgw key create ...` |
| `429 rate_limit_exceeded` | Vượt RPM | Tăng `--rpm` khi tạo key |
| `429 insufficient_quota` | Hết budget | Tăng `--budget` |
| Redis down | Rate limit bị tắt (graceful) | Kiểm tra `REDIS_HOST` |
| Key revoked vẫn dùng được | Cache 10 phút | Revoke đã invalidate cache ngay, chờ tối đa 10p nếu gọi tắt |
