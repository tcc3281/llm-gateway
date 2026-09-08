# 🗺️ Lộ Trình Phát Triển Tính Năng (Future Roadmap)

Tài liệu này ghi nhận định hướng kiến trúc và các tính năng chiến lược sẽ được tập trung phát triển trong các phiên bản tiếp theo của **LLM Gateway** (từ `v0.3.0` trở đi).

---

## 📌 1. Tính Năng Trọng Tâm 1: Virtual Models & Dynamic Routing (Mô Hình Ảo & Định Tuyến Động)

### 🎯 Bài toán thực tế
Trong môi trường production, các microservices hoặc ứng dụng của doanh nghiệp thường phải hardcode tên model cụ thể (ví dụ: `model: "gpt-4o"` hoặc `model: "claude-3-5-sonnet"`). Điều này dẫn đến các điểm nghẽn nghiêm trọng:
- Khi muốn đổi sang model rẻ hơn hoặc nâng cấp model mới (ví dụ từ `gpt-4o` sang `gpt-4o-mini`, hoặc thử nghiệm `gemini-1.5-pro`): Kỹ sư phải sửa code, tạo PR, build lại Docker image và deploy lại toàn bộ ứng dụng client.
- Khi một nhà cung cấp gặp sự cố hoặc cạn quota (Rate Limit 429): Toàn bộ hệ thống client bị gián đoạn hoạt động.

### 💡 Giải pháp Mô Hình Ảo (Virtual Model Aliasing)
LLM Gateway đóng vai trò làm tầng trừu tượng hóa mô hình (Model Abstraction Layer):
- Ứng dụng client chỉ cần gọi một **tên mô hình ảo (Model Alias)** cố định mang tính nghiệp vụ:
  - `production-fast-chat` (Dành cho chatbot phản hồi nhanh, tác vụ nhẹ).
  - `production-coder` (Dành cho sinh mã nguồn, fix bug).
  - `production-reasoning` (Dành cho phân tích logic sâu, giải toán, tài chính).
  - `production-embedding` (Dành cho vector search, RAG).
- Toàn bộ việc ánh xạ từ Model Alias sang Model thật và Provider cụ thể được quản lý tập trung trên Gateway:
  ```
  Client Request: model = "production-fast-chat"
         │
         ▼
  [LLM Gateway Virtual Model Router]
         │
         ├── 90% traffic  ──► OpenAI: gpt-4o-mini
         └── 10% traffic  ──► Anthropic: claude-3-5-haiku (Canary evaluation)
  ```
- **Lợi ích**: Khi muốn đổi sang model khác, Quản trị viên chỉ cần thay đổi cấu hình trên Admin Web UI hoặc chạy 1 lệnh CLI: **Toàn bộ hệ thống production chuyển đổi sang model mới ngay lập tức trong 0 giây downtime**, không cần sửa một dòng code client nào.

### 🚀 Các năng lực nâng cao đi kèm
1. **Fallback Chains (Chuỗi dự phòng tự động)**:
   - Cấu hình thứ tự ưu tiên: `Primary: gpt-4o` $\rightarrow$ `Secondary: claude-3-5-sonnet` $\rightarrow$ `Backup: gemini-1.5-pro`.
   - Nếu Primary trả về mã lỗi `429 Too Many Requests`, `502 Bad Gateway` hoặc timeout quá 10 giây, Gateway tự động chuyển tiếp request sang Secondary trong suốt với người dùng.
2. **Traffic Splitting / A/B Canary Testing**:
   - Cho phép chia tỷ lệ phần trăm lưu lượng (ví dụ: 80% traffic vào model hiện tại, 20% traffic vào model mới) để đo lường độ trễ thực tế, tỷ lệ lỗi và chi phí trước khi quyết định cutover hoàn toàn.
3. **Cost & Latency-Aware Smart Routing**:
   - Tự động định tuyến request đơn giản tới model rẻ hơn và request phức tạp (dài hơn 4k token hoặc có ảnh) tới model mạnh hơn.

---

## 📌 2. Tính Năng Trọng Tâm 2: Tối Ưu Hóa Bộ Công Cụ CLI (`llmgw`)

### 🎯 Mục tiêu
Nâng cấp công cụ dòng lệnh `llmgw` thành một trợ thủ đắc lực cho DevOps, SRE và kỹ sư quản trị hạ tầng, cho phép cấu hình và debug Gateway mà không bắt buộc phải mở trình duyệt web.

### 💡 Các cải tiến chi tiết
1. **Quản lý Profile đa môi trường (Multi-Environment Profiles)**:
   - Hỗ trợ lưu trữ thông tin kết nối và API Key của nhiều môi trường:
     ```bash
     llmgw profile add prod --url https://gateway.internal.company.com --key sk-admin-...
     llmgw profile add local --url http://localhost:8081 --key sk-admin-secret...
     llmgw profile use prod
     ```
2. **Quản trị Mô hình ảo & Bảng giá trực tiếp qua CLI**:
   - Tạo và cập nhật Model Alias tức thì:
     ```bash
     llmgw alias create fast-chat --target gpt-4o-mini --fallback claude-3-5-haiku
     llmgw alias update fast-chat --target gemini-1.5-flash
     llmgw alias list
     ```
3. **Vận hành Khóa API & Tenant nhanh**:
   - Cấp khóa mới, nạp tiền hoặc tra cứu hạn mức chỉ với 1 dòng lệnh:
     ```bash
     llmgw keys create --name "slack-bot" --user dev@company.com --budget 100 --rpm 200
     llmgw tenants top-up <tenant-id> 500 --ref "WIRE-BANK-TX982" --notes "Q3 Budget"
     ```
4. **Giám sát & Debug thời gian thực (Live Tail Logs)**:
   - Truy xuất log yêu cầu dạng bảng có màu hoặc stream log thời gian thực:
     ```bash
     llmgw logs tail --status 5xx --follow
     llmgw stats summary --range 24h
     ```
5. **Đóng gói & Phân phối tối ưu**:
   - Đóng gói single binary độc lập (Go native hoặc PyInstaller/Nuitka) hoặc cài đặt nhanh qua Homebrew / Docker / `pipx` / `uv`.
   - Bổ sung Shell Autocompletion cho `bash`, `zsh`, `fish`.

---

## 📌 3. Tính Năng Trọng Tâm 3: Hệ Thống Plugin Đa Cấp Độ (Kong-like Plugin Architecture)

### 🎯 Bài toán & Tầm nhìn kiến trúc
Giống như kiến trúc mở rộng nổi tiếng của **Kong API Gateway**, thay vì nhồi nhét mọi logic nghiệp vụ vào lõi Gateway khiến mã nguồn phình to và khó bảo trì, LLM Gateway sẽ chuyển dịch sang **Kiến trúc Plugin Module hóa (Pluggable Pipeline Engine)**:
- Mỗi tính năng kiểm soát, tối ưu, bảo mật hoặc quan sát được đóng gói thành một **Plugin độc lập**.
- Quản trị viên có thể **bật (activate), tắt (deactivate)** hoặc cấu hình tham số cho từng tính năng tức thì qua Web UI hoặc CLI mà không cần restart Gateway.
- Vòng đời can thiệp linh hoạt (Plugin Lifecycle Hooks):
  ```
  Client Request 
       │
       ▼
  [Pre-Auth Hook] ──► [Post-Auth / IAM Hook] ──► [Pre-Routing / Guardrails]
                                                        │
  Client Response ◄── [Audit / Post-Upstream] ◄── [Upstream LLM Provider]
  ```

### 🎚️ Cơ chế Gắn Cấu Hình Đa Cấp Độ (Multi-Level Scoping & Inheritance)
Khác với các hệ thống thông thường chỉ cho phép cấu hình Global, Plugin trong LLM Gateway có thể được gắn (apply) và tùy biến tham số ở nhiều tầng phân cấp khác nhau:

```
  ┌─────────────────────────────────────────────────────────────────────┐
  │ 1. GLOBAL (Áp dụng toàn bộ Gateway)                                │
  │    └─► 2. PROVIDER (Áp dụng cho mọi model của OpenAI, Gemini, ...)  │
  │         └─► 3. MODEL (Áp dụng cho model cụ thể: gpt-4o, claude-3-5) │
  │              └─► 4. TENANT / GROUP (Áp dụng theo tổ chức / nhóm)    │
  │                   └─► 5. ROLE / IAM (Áp dụng theo vai trò user)     │
  │                        └─► 6. USER (Áp dụng theo từng người dùng)   │
  │                             └─► 7. API KEY (Áp dụng riêng cho khóa) │
  └─────────────────────────────────────────────────────────────────────┘
  (Độ ưu tiên ghi đè: API Key > User > Role/IAM > Tenant/Group > Model > Provider > Global)
```

*Ví dụ vận hành thực tế*:
- **Cấp Global**: Kích hoạt plugin `pii-masking` để đảm bảo không dữ liệu nhạy cảm nào bị lọt ra ngoài, và plugin `audit-log` để lưu vết tuân thủ.
- **Cấp Tenant (Phòng Marketing)**: Bật plugin `budget-control` giới hạn 1,000 USD/tháng.
- **Cấp Role (`Intern`)**: Gán plugin `rate-limiting` giới hạn 20 RPM và chặn không cho gọi các model siêu đắt tiền.
- **Cấp Model (`o1-preview`)**: Gán plugin `rate-limiting` 5 requests/phút để tránh quá tải hạn mức OpenAI.
- **Cấp API Key (`sk-gw-batch-worker`)**: Ghi đè tham số `rate-limiting` lên 1,000 RPM và bật plugin `semantic-cache` với ngưỡng tương đồng $0.98$.

### 🧩 Danh Mục Plugin Cốt Lõi Dự Kiến (First-Party Plugin Catalog)

1. **Traffic & Quota Management**:
   - `rate-limiting`: Kiểm soát lưu lượng theo RPM (Requests/phút), TPM (Tokens/phút), hoặc Concurrent Requests theo từng Key/User/IP.
   - `budget-control`: Quản lý hạn mức chi phí (USD) theo chu kỳ Giờ / Ngày / Tháng, tự động cảnh báo hoặc chặn request khi vượt ngưỡng.
2. **Performance & Caching**:
   - `semantic-cache`: Bộ đệm thông minh sử dụng Vector Embeddings (Cosine Similarity $\ge 0.95$), trả lời tức thì các câu hỏi tương đồng ngữ nghĩa, tiết kiệm tới 80% chi phí và giảm độ trễ về dưới 10ms.
   - `request-hedging`: Gửi đồng thời tới 2 upstream provider cho các request ưu tiên cao (VIP), nhận kết quả từ bên có TTFT (Time-to-first-token) nhanh nhất.
3. **Security, Privacy & Guardrails**:
   - `pii-masking`: Tự động phát hiện và làm mờ dữ liệu nhạy cảm (Email, SĐT, Số thẻ ngân hàng, CCCD/CMND, API Keys) trước khi gửi tới LLM bên ngoài.
   - `prompt-guard`: Ngăn chặn tấn công Prompt Injection, Jailbreak, và kiểm duyệt nội dung độc hại (Toxicity / Harmful content).
4. **Resilience & Routing**:
   - `retry-circuit-breaker`: Tự động thử lại hoặc ngắt mạch (Circuit Breaker) khi upstream trả về 429/500 liên tiếp.
   - `header-transform`: Thêm/sửa/xóa các HTTP header gửi đến upstream hoặc trả về cho client.
5. **Observability & Analytics**:
   - `langfuse-exporter`: Tự động đồng bộ traces, spans và metadata sang nền tảng Langfuse.
   - `audit-trail`: Xuất dữ liệu gọi API sang Kafka, ElasticSearch hoặc S3 phục vụ kiểm toán nội bộ.

---

## 📅 Lộ Trình Triển Khai Đề Xuất (Release Plan)

| Phiên bản | Mục tiêu chiến lược | Các tính năng trọng tâm | Trạng thái |
| :--- | :--- | :--- | :---: |
| **v0.2.x** | Enterprise Foundation | Distributed Architecture, AWS IAM, Multi-tenancy, Alerting, i18n, MIT License | **Đã hoàn thành** |
| **v0.3.0** | **Virtual Models & Routing** | Virtual Model Aliasing, Fallback Chains, Model Mapping CLI & Web UI | *Kế hoạch kế tiếp* |
| **v0.3.1** | **CLI Tooling Optimization** | Quản lý Profile đa môi trường, Live Tail Logs, Package standalone binary | *Kế hoạch* |
| **v0.4.0** | **Plugin Core Architecture** | Core Plugin Engine (Lifecycle Hooks), Multi-Level Scoping (Key/User/Tenant/Global), Plugins: `rate-limiting` (RPM/TPM), `budget-control` | *Kế hoạch* |
| **v0.5.0** | **Advanced Intelligence Plugins** | Plugins: `semantic-cache` (Vector Embedding), `pii-masking`, `prompt-guard`, `langfuse-exporter` | *Nghiên cứu* |
| **v1.0.0** | **Enterprise Production Ready** | Custom WASM/Go Plugin SDK cho bên thứ 3, High-Throughput Zero-Copy Engine | *Mục tiêu dài hạn* |
