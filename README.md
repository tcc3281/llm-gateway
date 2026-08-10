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
> docker exec -it llm_gateway llmgw provider set --provider openai --api-key sk-proj-...
> docker exec -it llm_gateway llmgw provider set --provider anthropic --api-key sk-ant-...
> docker exec -it llm_gateway llmgw provider set --provider gemini --api-key AIzaSy...
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

Bộ công cụ CLI **`llmgw`** giúp quản trị Gateway cực kỳ nhanh chóng qua Terminal (tương tự như `docker`, `kubectl`). File binary `llmgw` đã được build sẵn trong Docker container.

### 1. Quản lý Người Dùng & Khởi tạo API Key
```bash
# Tạo User mới
docker exec -it llm_gateway llmgw user create \
  --email dev@company.com \
  --name "Developer 1" \
  --role user

# Tạo API Key cho User (với Budget Limit $50 USD và 100 requests/phút)
docker exec -it llm_gateway llmgw key create \
  --user-id <USER_UUID> \
  --name "Production App Key" \
  --budget 50.0 \
  --rpm 100

# Liệt kê Users / API Keys & Thu hồi Key (revoke)
docker exec -it llm_gateway llmgw user list
docker exec -it llm_gateway llmgw key list
docker exec -it llm_gateway llmgw key revoke --id <KEY_UUID>
```

### 2. Quản lý Provider API Keys (lưu trong DB)
```bash
# Cấu hình key cho từng Provider (OpenAI / Anthropic / Gemini)
docker exec -it llm_gateway llmgw provider set \
  --provider openai \
  --api-key sk-proj-...

# Xem danh sách keys (secret được che giấu)
docker exec -it llm_gateway llmgw provider list

# Xoá key của một Provider
docker exec -it llm_gateway llmgw provider delete --provider openai
```
*Nếu client muốn dùng upstream key riêng cho từng request, gửi header `x-upstream-api-key` — nó sẽ override key trong DB.*

### 3. Quản lý Models & Pricing Rate
```bash
# Xem danh sách Models & Giá hiện tại dạng Bảng
docker exec -it llm_gateway llmgw model list

# Thêm/Cập nhật Model & Giá mới (Tính theo 1M token)
docker exec -it llm_gateway llmgw model add \
  --provider openai \
  --name gpt-4o \
  --prompt-price 2.50 \
  --completion-price 10.00
```

### 4. Xem Báo Cáo Thống Kê Usage & Telemetry (Stats)
```bash
# Xem báo cáo Token, Cost, Latency trung bình nhóm theo giờ
docker exec -it llm_gateway llmgw stats --interval hour
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
