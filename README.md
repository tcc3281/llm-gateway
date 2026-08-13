# LLM Gateway (Go + Docker)

Một **LLM Gateway** hiệu năng cao được phát triển bằng ngôn ngữ **Go**, hỗ trợ Proxy & Protocol Adapter cho 3 nhà cung cấp LLM chính: **OpenAI**, **Anthropic**, và **Google Gemini**. Gateway cung cấp chuẩn đầu vào/đầu ra tương thích với cả **OpenAI API** (`/v1/chat/completions`) và **Anthropic API** (`/v1/messages`).

---

## 🌟 Kiến Trúc 2 Port Độc Lập

Hệ thống được thiết kế với sự phân tách rõ ràng giữa lưu lượng người dùng (Public) và quản trị (Admin):

1. **Port 8080 — Public LLM Gateway**:
   * Phục vụ các API Proxy công khai cho Client & SDKs: `POST /v1/chat/completions`, `POST /v1/messages`, `GET /v1/models`.
   * **Xử lý Real Stream & Upstream Forwarding**: Gateway chuyển tiếp request thật tới OpenAI / Anthropic / Gemini, dịch SSE real-time, đo latency (TTFT), đếm token thật và tính toán cost lưu vào DB.

2. **Port 8081 — Admin Management Server**:
   * Phục vụ các API Quản trị & Cấu hình: `POST /admin/users`, `POST /admin/keys`, `GET/POST/DELETE /admin/models`, `GET /admin/analytics/stats`.
   * Chỉ phục vụ trong mạng nội bộ (Internal/Admin network).

---

## 🔐 Authentication, Rate Limiting & Quota

Gateway áp dụng các lớp bảo mật và kiểm soát lưu lượng nghiêm ngặt:

### 1. Authentication (Xác thực)
- **Public API (Port 8080)**: Bắt buộc gửi đính kèm API Key của Gateway (`Authorization: Bearer sk-gw-...` hoặc `x-api-key: sk-gw-...`). API Key này được cấp phát cho từng user, sử dụng thuật toán hash an toàn. Key được Cache qua Redis để đảm bảo tốc độ xác thực `< 1ms`. Nếu thiếu hoặc sai key, trả về lỗi `HTTP 401 Unauthorized`.
- **Admin API (Port 8081)**: Bắt buộc gửi header `X-Admin-API-Key`. Key này được cấu hình trong file `.env` của hệ thống (biến `ADMIN_API_KEY`).

### 2. Rate Limiting & Budget Quota
- **Rate Limit (RPM)**: Được kiểm soát bằng Redis Token Bucket. Giới hạn số lượng request trên phút (Requests Per Minute) được cấu hình riêng cho từng API Key. Vượt quá sẽ bị chặn với lỗi `HTTP 429 Too Many Requests (Rate limit exceeded)`.
- **Budget Quota**: Mỗi API Key có một ngân sách tối đa bằng USD. Mỗi request thành công sẽ được tính tiền dựa trên lượng Token (Prompt/Completion) tiêu thụ và trừ dần vào ngân sách. Nếu vượt ngân sách, Gateway trả lỗi `HTTP 429 Too Many Requests (Insufficient quota)`.

---

## 📊 Metrics, Telemetry & Analytics

Hệ thống thu thập đo lường số liệu của từng request mà không làm chậm trải nghiệm của User nhờ cơ chế **Asynchronous Worker Pool** (đẩy log vào Queue và batch insert vào PostgreSQL). Các nhóm Metrics bao gồm:

1. **Performance Metrics (Hiệu suất & Độ trễ)**:
   - `TTFTMs (Time To First Token)`: Thời gian phản hồi tính từ lúc gửi request đến khi nhận được chunk token đầu tiên (rất quan trọng với Streaming).
   - `TotalLatencyMs`: Tổng thời gian hoàn thành 1 request.
2. **Usage & Cost Metrics (Khối lượng & Chi phí)**:
   - `PromptTokens` / `CompletionTokens` / `TotalTokens`: Số lượng token đầu vào/ra thực tế được LLM trả về.
   - `Cost (USD)`: Chi phí được tính toán realtime theo bảng giá (Pricing) của Model.
3. **Routing & Metadata (Phân loại & Định tuyến)**:
   - Ghi nhận `Provider` (openai/anthropic/gemini) và `Model` đích thực sự xử lý request.
   - Định dạng inbound API được client gọi (`InboundFormat`).
4. **Reliability Metrics (Độ tin cậy)**:
   - `StatusCode`: HTTP Status Code trả về.
   - `ErrorMessage`: Thông báo lỗi (nếu request thất bại) để dễ dàng trace lỗi hệ thống.

---

## 📊 Dashboard JSON API (cho Dashboard Tuỳ Chỉnh)

Endpoint dành cho dashboard riêng — trả **payload JSON tổng hợp** đọc trực tiếp từ bảng `usage_logs`, không cần Prometheus/Grafana:

```bash
curl "http://localhost:8081/admin/analytics/dashboard?interval=hour" \
  -H "X-Admin-API-Key: sk-admin-secret-key-change-me"
```

**Query params:** `interval` (`hour`/`day`, mặc định `hour`), `user_id` (lọc theo user), `from`/`to` (RFC3339, mặc định 24h gần nhất), `limit` (số recent request, mặc định 20, tối đa 200).

**Response:**
```json
{
  "from": "...", "to": "...", "interval": "hour",
  "totals": {
    "requests": 120, "cost_usd": 1.45,
    "prompt_tokens": 9000, "completion_tokens": 2400,
    "avg_ttft_ms": 412.3, "avg_latency_ms": 1800.1,
    "errors": 3, "error_rate": 0.025
  },
  "time_series": [
    { "time_bucket": "2026-08-10T21:00:00Z", "requests": 40, "cost_usd": 0.5,
      "prompt_tokens": 3000, "completion_tokens": 800, "errors": 1,
      "avg_ttft_ms": 400.1, "avg_latency_ms": 1700.0 }
  ],
  "by_model": [
    { "provider": "openai", "model": "gpt-4o", "requests": 80, "cost_usd": 1.2,
      "prompt_tokens": 6000, "completion_tokens": 1600, "avg_latency_ms": 1500.0 }
  ],
  "by_user": [
    { "user_id": "...", "email": "dev@company.com", "requests": 100, "cost_usd": 1.3,
      "prompt_tokens": 8000, "completion_tokens": 2000 }
  ],
  "recent": [
    { "created_at": "...", "user_id": "...", "provider": "openai", "model": "gpt-4o",
      "status_code": 200, "error_message": "", "cost_usd": 0.02,
      "ttft_ms": 350, "latency_ms": 1400 }
  ]
}
```

Mỗi lần gọi đều query `usage_logs` mới — không cache, dữ liệu realtime theo async telemetry worker.

---

## 🖥️ Dashboard Admin (Web UI)

Gateway phục vụ sẵn một dashboard admin **React + Vite + TS** (theme Obsidian Telemetry) ngay trên cổng admin `:8081` qua `go:embed` — không cần deploy riêng.

### Truy cập

```bash
# 1. Build frontend (chạy lần đầu hoặc khi sửa web/)
make web-build        # tương đương: cd web && npm run build

# 2. Dựng & chạy stack (tự chạy web-build trước khi build image)
make build            # docker-compose up -d --build

# 3. Mở trình duyệt
#    http://localhost:8081/
```

Đăng nhập bằng **admin API key** (biến `ADMIN_API_KEY` trong `.env`). Key được lưu ở browser localStorage và gửi trong header `X-Admin-API-Key` cho mọi request `/admin/*`.

### Trang chính

| Route          | Chức năng                                                        |
| -------------- | ---------------------------------------------------------------- |
| `/login`       | Nhập admin key (verify qua `GET /admin/users`)                    |
| `/dashboard`   | KPI requests/cost/tokens/errors + time-series + by-model + by-user + recent |
| `/users`       | Danh sách / tạo / sửa / xoá user                                  |
| `/keys`        | API keys: lọc theo user, tạo (hiện secret 1 lần), sửa budget/rpm/expiry, revoke/reactivate |
| `/providers`   | Set / rotate / xoá provider key (secret không bao giờ trả về browser) |
| `/models`      | Bảng giá model: add / edit / delete / sync từ LiteLLM catalog      |
| `/logs`        | Usage logs: filter (provider/model/user/status/format/thời gian) + phân trang |
| `/settings`    | Health, ports, DB/Redis thông tin redacted, cờ bảo mật             |

### Phát triển (hot reload)

```bash
make up          # admin server :8081 đang chạy
make web-dev     # Vite dev server http://localhost:5173, proxy /admin → :8081
```

### Thư mục

- `web/` — mã nguồn React (Vite build ra `internal/web/dist`).
- `internal/web/` — `embed.go` nhúng `dist` vào binary + placeholder `dist/index.html` để `go build` luôn chạy được khi chưa build frontend.

---

## 🔄 Inbound / Outbound Protocol Standards (Chuẩn I/O)

Gateway đóng vai trò làm Universal Adapter, hỗ trợ 2 chuẩn giao tiếp phổ biến nhất hiện nay:

### Chuẩn 1: OpenAI Compatible API (`/v1/chat/completions`)
Client (Ứng dụng/SDK của user) có thể dùng chuẩn OpenAI để gọi **bất kỳ model nào** (OpenAI, Anthropic, Gemini).
- **Input (Request Payload)**: Tuân thủ chuẩn OpenAI (`model`, `messages`, `temperature`, `stream`...).
- **Output (Response Payload / SSE Stream)**: Gateway sẽ trả về cục JSON chuẩn OpenAI, hoặc stream SSE với định dạng chuẩn của OpenAI (`data: {"choices": [{"delta": {"content": "..."}}]}`).
- **Quá trình Adapter xử lý**:
  - Nếu gọi model Anthropic (ví dụ: `claude-3-5-sonnet...`): Gateway tự map payload sang chuẩn Anthropic Messages API -> Gọi Anthropic -> Nhận luồng SSE của Anthropic -> Parse và map ngược luồng SSE đó về chuẩn OpenAI SSE -> Trả cho Client.
  - Tương tự với Gemini.

### Chuẩn 2: Anthropic Compatible API (`/v1/messages`)
Client có thể gọi vào route này nếu hệ thống Client được thiết kế dùng SDK Anthropic nguyên bản.
- **Input/Output**: Tuân thủ chuẩn cấu trúc Anthropic API (`max_tokens`, `messages`, `system`...).

*Bất kể chuẩn nào, Gateway cũng xử lý minh bạch (transparent) lỗi từ upstream (ví dụ mapping lỗi `502`, `400` của nhà cung cấp về chuẩn OpenAI Error format).*

---

## 🧠 Streaming & Reasoning

Gateway hỗ trợ streaming (SSE) và reasoning/thinking xuyên chuẩn (cross-provider). Cơ chế bật reasoning là **passthrough tham số gốc của provider** — không hardcode danh sách model reasoning. Nội dung reasoning được **forward về client** (không bị rớt).

### 1. OpenAI chuẩn — `reasoning_effort`

Gửi `reasoning_effort` (low/medium/high) để bật reasoning, kết hợp `stream: true`:

```bash
curl -s http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer <API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash",
    "messages": [{"role": "user", "content": "1+1=?"}],
    "reasoning_effort": "low",
    "stream": true
  }'
```

- Non-stream: nhận `message.reasoning_content` (chuỗi suy luận) trước `content`.
- Stream: nhận `delta.reasoning_content` trong các chunk đầu, rồi `delta.content`.
- Gateway tự gửi `stream_options.include_usage` lên upstream, nên usage (kể cả `reasoning_tokens`) vẫn về trong chunk cuối để tính cost/telemetry chính xác.

### 2. Anthropic chuẩn — `thinking`

Gửi `thinking: {type: "enabled", budget_tokens: ...}`:

```bash
curl -s http://localhost:8080/v1/messages \
  -H "x-api-key: <API_KEY>" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-5",
    "max_tokens": 4096,
    "messages": [{"role": "user", "content": "Giải thích thuật toán merge sort"}],
    "thinking": {"type": "enabled", "budget_tokens": 2048}
  }'
```

- Non-stream: trả về block `{"type":"thinking","thinking":"..."}` đứng trước block text.
- Stream: lifecycle đầy đủ `message_start` → `content_block_start` (thinking) → `content_block_delta` (`thinking_delta`) → `content_block_stop` → `content_block_start` (text) → ... → `message_stop`.
- `budget_tokens` client gửi được tôn trọng khi đi qua gateway; nếu gọi cross-format (OpenAI-inbound → Anthropic), gateway tự suy budget từ `max_tokens`/`reasoning_effort`.

### 3. Cross-format — OpenAI-inbound gọi Gemini thinking

Model Gemini trong catalog (vd `gemini-2.5-flash`) gọi bằng chuẩn OpenAI + `reasoning_effort` → Gateway tự bật `thinkingConfig.includeThoughts` và tách thought text khỏi nội dung trả về.

### 4. Chi phí reasoning

`reasoning_tokens` đã được tính vào cost (theo completion rate) và ghi vào `usage_logs` + Dashboard — không cần cấu hình thêm.

## ⚠️ Known Limitations

- **Tool / Function calling hỗ trợ qua `tools` + `tool_choice` hiện đại** (OpenAI & Anthropic wire). Field **legacy `functions` / `function_call`** (dạng cũ của OpenAI) **vẫn bị bỏ qua** — hãy dùng `tools: [{type:"function", function:{...}}]`.

---

## 🚀 Hướng Dẫn Chạy Hệ Thống Với Docker

Hệ thống được đóng gói bằng Docker Compose (Gateway, Postgres 16, Redis 7).

### Step 1: Chuẩn bị file `.env`
```bash
cp .env.example .env
```
`.env` chỉ chứa cấu hình hệ thống (server, database, redis, admin secret, master key mã hoá):
```env
ADMIN_API_KEY=sk-admin-secret-key-12345
PROVIDER_MASTER_KEY=change-me-to-a-long-random-passphrase   # optional, mã hoá key tại rest
```

> ⚠️ **Provider API Keys (OpenAI/Anthropic/Gemini) KHÔNG đặt trong `.env` nữa.** Chúng được quản lý stateful trong database (bảng `provider_keys`) qua CLI hoặc Admin API:
> ```bash
> uv run --project cli llmgw provider set --provider openai --api-key sk-proj-...
> uv run --project cli llmgw provider set --provider anthropic --api-key sk-ant-...
> uv run --project cli llmgw provider set --provider gemini --api-key AIzaSy...
> ```

### Step 2: Khởi chạy Stack với Docker Compose
```bash
docker-compose up -d --build
```

Kiểm tra Healthcheck (Gateway chạy trên 8080 và 8081):
```bash
curl http://localhost:8081/health
```

---

## 💻 Hướng Dẫn Sử Dụng Bộ CLI Quản Trị (`llmgw`)

Bộ công cụ CLI **`llmgw`** (Python `click`) giúp quản trị Gateway qua Terminal (tương tự như `docker`, `kubectl`). CLI chạy **trên host** từ thư mục repo bằng `uv` và gọi admin API (port 8081):

```bash
# Yêu cầu: uv đã cài trên host (https://docs.astral.sh/uv/). Không cần build gì thêm.
uv run --project cli llmgw health
```

Config (URL + admin key) lấy từ env `GATEWAY_URL`/`ADMIN_API_KEY`, hoặc config file `~/.config/llmgw/config.json`, hoặc mặc định `http://localhost:8081`. Chạy `llmgw config set` lần đầu để lưu admin key.

### 1. Quản lý Người Dùng & Khởi tạo API Key
```bash
# Tạo User mới
uv run --project cli llmgw user create \
  --email dev@company.com \
  --name "Developer 1" \
  --role user

# Tạo API Key cho User (với Budget Limit $50 USD và 100 requests/phút)
uv run --project cli llmgw key create \
  --user-id <USER_UUID> \
  --name "Production App Key" \
  --budget 50.0 \
  --rpm 100

# Liệt kê Users / API Keys & Thu hồi Key (revoke)
uv run --project cli llmgw user list
uv run --project cli llmgw key list
uv run --project cli llmgw key revoke --id <KEY_UUID>
```

### 2. Quản lý Provider API Keys (lưu trong DB)
```bash
# Cấu hình key cho từng Provider (OpenAI / Anthropic / Gemini)
uv run --project cli llmgw provider set \
  --provider openai \
  --api-key sk-proj-...

# Xem danh sách keys (secret được che giấu)
uv run --project cli llmgw provider list

# Xoá key của một Provider
uv run --project cli llmgw provider delete --provider openai
```
*Nếu client muốn dùng upstream key riêng cho từng request, gửi header `x-upstream-api-key` — nó sẽ override key trong DB.*

#### 2.1. Custom OpenAI / Anthropic-Compatible Providers
Ngoài 3 provider mặc định (openai/anthropic/gemini), bạn có thể thêm **provider compatible** bất kỳ
(OpenRouter, Groq, vLLM, Ollama, DeepSeek, …) bằng cách cấu hình `base_url` + `protocol`.

- `base_url`: endpoint gốc của upstream. Gateway tự nối `/chat/completions` (protocol `openai`)
  hoặc `/v1/messages` (protocol `anthropic`).
- `protocol`: wire protocol gateway sẽ nói với `base_url` đó — `openai` (mặc định) hoặc `anthropic`.
- Provider mặc định **không** được đặt `base_url` (giữ URL cứng trong gateway).

**Qua CLI:**
```bash
uv run --project cli llmgw provider set \
  --provider openrouter \
  --base-url https://openrouter.ai/api/v1 \
  --protocol openai \
  --api-key sk-or-v1-...

uv run --project cli llmgw provider set \
  --provider deepseek \
  --base-url https://api.deepseek.com \
  --protocol openai \
  --api-key sk-...
```

**Qua Web UI (Admin Dashboard → Providers):** chọn `custom (compatible)…`, nhập tên provider,
`base_url`, `protocol` và API key.

**Cách gọi:** model dạng prefix `tênprovider/tênmodel` — segment đầu tiên là tên provider đã cấu hình:
```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-gw-..." \
  -H "Content-Type: application/json" \
  -d '{"model": "openrouter/anthropic/claude-3.5-sonnet", "messages": [{"role":"user","content":"hi"}]}'
```
Muốn tính cost chính xác, đăng ký giá model dưới **tên provider custom** đó (Models → Add / `llmgw model add --provider openrouter --name ...`).

#### 2.2. Routing quyết định theo registry model (không đoán prefix)
Gateway quyết định upstream theo **thứ tự ưu tiên** sau:
1. **Prefix `tênprovider/tênmodel`** — client chọn tường minh một provider custom (đã cấu hình `base_url`).
2. **Catalog model** — model đã biết thuộc provider nào thì route tới provider đó, lấy từ 2 nguồn gộp lại:
   - **`model_pricings`** (trang Models / `llmgw model add`) — model **đã đăng ký giá** dưới provider nào; đây là nguồn routing chính xác nhất, chỉ cần note model trong Models là gateway biết gửi đi đâu.
   - **`provider_models`** (tự phát hiện) — danh sách model lấy từ API `/models` của mỗi provider khi bấm **Discover models** trên Models / `llmgw model discover`, chạy tự động mỗi giờ. Không cần fix cứng model nào trong code — thêm provider mới là catalog tự có.
3. **Model không nằm trong catalog** → **lỗi rõ ràng 404 `model_not_found`**, gateway **không** đoán upstream theo prefix tên nữa.

Ví dụ — model của Google như `gemma-4-26b-a4b-it` **không** bắt đầu bằng `gemini`. Trước đây gateway đoán theo prefix và gửi sang OpenAI → nhận lỗi 404. Giờ đăng ký/discover nó vào đúng provider thì gateway gửi đúng:
```bash
uv run --project cli llmgw model add \
  --provider gemini --name gemma-4-26b-a4b-it \
  --prompt-price 0.25 --completion-price 1.00
```
Sau đó gọi model bình thường (không cần prefix) — gateway tự route tới Gemini:
```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-gw-..." -H "Content-Type: application/json" \
  -d '{"model": "gemma-4-26b-a4b-it", "messages": [{"role":"user","content":"hi"}]}'
```
Lệnh `llmgw model discover` (hoặc nút **Discover models** trên trang Models) quét toàn bộ provider đã cấu hình và ghi model vào catalog `provider_models`.

### 3. Quản lý Models & Pricing Rate
```bash
# Xem danh sách Models & Giá hiện tại dạng Bảng
uv run --project cli llmgw model list

# Xem toàn bộ catalog routable: discovered (provider_models) + registered pricing
uv run --project cli llmgw model catalog
uv run --project cli llmgw model catalog --provider gemini

# Thêm/Cập nhật Model & Giá mới (Tính theo 1M token)
uv run --project cli llmgw model add \
  --provider openai \
  --name gpt-4o \
  --prompt-price 2.50 \
  --completion-price 10.00

# Xoá pricing rule — chấp nhận UUID, prefix, hoặc tên model
uv run --project cli llmgw model delete --id gpt-4o
```
> Phân biệt rõ 2 lệnh model:
> - `llmgw model sync` — pull **giá** từ LiteLLM catalog vào `model_pricings` (**chỉ cho tính cost**, không ảnh hưởng routing).
> - `llmgw model discover` — fetch API `/models` của từng provider vào `provider_models` (**routing catalog**; model ở đây mới routable được). Xem danh sách bằng `llmgw model catalog`.

### 4. Xem Báo Cáo Thống Kê Usage & Telemetry (Stats)
```bash
# Xem báo cáo Token, Cost, Latency trung bình nhóm theo giờ
uv run --project cli llmgw stats --interval hour

# Xem usage logs gần nhất (filter theo status/model/provider/user)
uv run --project cli llmgw logs --limit 50 --status 2xx
```

### 5. Smoke-test Public Proxy — `llmgw chat`
Gọi trực tiếp public proxy (port 8080) bằng model + API key user — dùng để verify routing E2E sau khi đổi provider config:
```bash
# Gọi OpenAI-inbound (mặc định)
uv run --project cli llmgw chat \
  --api-key sk-gw-... --model gemma --message "Hi"

# Stream SSE realtime + hiện reasoning content (Gemini thinking)
uv run --project cli llmgw chat \
  --api-key sk-gw-... --model gemini-2.5-flash --stream --show-reasoning

# Gọi Anthropic-inbound (cross-format qua /v1/messages)
uv run --project cli llmgw chat --api-key sk-gw-... \
  --model claude-... --inbound anthropic

# Tool calling: inject test tool get_weather; OpenAI-inbound tự động round-trip tool result
uv run --project cli llmgw chat --api-key sk-gw-... \
  --model gemini-2.5-flash --tools
```
Có thể cấu hình `public_url`/`public_api_key` trong config file (hoặc env `GATEWAY_PUBLIC_URL`/`PUBLIC_API_KEY`) để khỏi truyền mỗi lần.

### 5b. Tool / Function Calling — proxy passthrough

Gateway **forward** `tools` / `tool_choice` lên upstream và **trả `tool_calls` về client**; **client chịu trách nhiệm execute tool** rồi gửi `tool_result` lại turn sau (gateway không execute tool). Vòng lặp chuẩn (OpenAI-inbound):

1. **Gửi** `tools` + prompt yêu cầu gọi tool:
   ```json
   {
     "model": "gemini-2.5-flash",
     "messages": [{"role": "user", "content": "What is the weather in Hanoi? Call the get_weather tool."}],
     "tools": [{"type": "function", "function": {
       "name": "get_weather",
       "description": "Get the current weather for a location",
       "parameters": {"type": "object", "properties": {"location": {"type": "string"}}, "required": ["location"]}
     }}]
   }
   ```
2. **Nhận** `choices[0].message.tool_calls` (non-stream) hoặc chunk cuối `delta.tool_calls` (stream), ví dụ:
   ```json
   {"id": "call_0", "type": "function", "function": {"name": "get_weather", "arguments": "{\"location\":\"Hanoi\"}"}}
   ```
3. **Execute** tool (client), rồi **gửi `tool_result`** turn sau:
   ```json
   {
     "model": "gemini-2.5-flash",
     "messages": [
       {"role": "user", "content": "What is the weather in Hanoi? Call the get_weather tool."},
       {"role": "assistant", "content": null, "tool_calls": [{"id": "call_0", "type": "function", "function": {"name": "get_weather", "arguments": "{\"location\":\"Hanoi\"}"}}]},
       {"role": "tool", "tool_call_id": "call_0", "content": "Sunny, 25°C"}
     ],
     "tools": [ { ... như trên ... } ]
   }
   ```

Lưu ý cross-format:
- **Anthropic-inbound** dùng wire tương đương: `tools:[{name,input_schema}]`, assistant trả block `{type:"tool_use",id,name,input}`, tool result là user message block `{type:"tool_result",tool_use_id,content}`.
- **Streaming**: tool-call arguments tới dạng fragment nên gateway **gộp lại và emit đầy đủ ở cuối stream** — text vẫn realtime, nhưng `tool_calls` chỉ xuất hiện ở chunk cuối trước `[DONE]`.
- **Gemini upstream**: không có tool_call_id → gateway synthesize `call_<n>`; id này chỉ có ý nghĩa trong lượt gọi đó (Gemini match functionResponse theo `name`).
- Chỉ hỗ trợ `tools`/`tool_choice` hiện đại; legacy `functions`/`function_call` vẫn bị bỏ qua.

### 6. Scripting & Automation — `--json`
Mọi lệnh đều hỗ trợ global `--json` để in raw JSON thay vì bảng rich — dùng để pipe vào `jq`/script:
```bash
uv run --project cli llmgw --json user list | jq '.[].email'
uv run --project cli llmgw --json logs --status 2xx --limit 5 | jq '.data[] | {model, cost}'
uv run --project cli llmgw --json model catalog | jq '.discovered[].model_name'
```

### 7. Tên thay cho ID
Các lệnh nhận ID (`key revoke --id`, `model delete --id`, `stats --user-id`, `key create --user-id`, `logs --user-id`) chấp nhận **UUID, prefix duy nhất, hoặc tên người dùng** (email / tên key / tên model):
```bash
uv run --project cli llmgw key revoke --id "Production App Key"
uv run --project cli llmgw stats --user-id dev@company.com
```

---

## 📖 Hướng Dẫn Sử Dụng Public API (Cho Client)

### Lấy Danh Sách Models (Public OpenAI Compatible API)
Endpoint yêu cầu xác thực `Authorization: Bearer sk-gw-...` (thiếu/sai → `401`). Danh sách được **lấy thật** từ các provider đã cấu hình key (OpenAI, Gemini) + các model đăng ký trong bảng `model_pricings` (gồm Anthropic — provider này không có API list model công khai), rồi **cache trong Redis 1 giờ**.
```bash
curl http://localhost:8080/v1/models \
  -H "Authorization: Bearer sk-gw-8f7a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a"
```

### Gọi LLM Theo Format OpenAI (`/v1/chat/completions`)

#### Gọi tới OpenAI (`gpt-4o`):
```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-gw-8f7a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a" \
  -d '{
    "model": "gpt-4o",
    "messages": [
      {"role": "user", "content": "Xin chào, hãy giới thiệu ngắn gọn về Go!"}
    ]
  }'
```

#### Gọi tới Anthropic (`claude-3-5-sonnet-20240620`) tự động map qua chuẩn OpenAI:
```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-gw-8f7a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a" \
  -d '{
    "model": "claude-3-5-sonnet-20240620",
    "messages": [
      {"role": "user", "content": "Hãy viết 1 đoạn Go code ngắn."}
    ]
  }'
```

---

## 🛠 Thư Mục Dự Án (Project Structure)

```text
.
├── cmd/
│   ├── cli/                      # Mã nguồn bộ công cụ llmgw CLI
│   └── gateway/                  # Entrypoint khởi tạo 2 HTTP Servers
├── internal/
│   ├── adapter/                  # Protocol translation (OpenAI, Anthropic, Gemini)
│   ├── analytics/                # Token counter, cost calculator, async logger
│   ├── auth/                     # API Key validation & hashing
│   ├── config/                   # Env configuration loader
│   ├── db/                       # Postgres & Redis repositories
│   ├── handler/                  # HTTP route handlers (Gateway & Admin)
│   ├── model/                    # Domain data structs
│   ├── provider/                 # Upstream HTTP transport client
│   └── service/                  # Core gateway orchestration logic
├── migrations/                   # SQL migration scripts
├── pkg/sse/                      # Server-Sent Events parser
├── Dockerfile                    # Multi-stage Docker build
├── docker-compose.yml            # Services orchestration (Gateway, Postgres, Redis)
└── README.md
```
