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

## 📌 3. Các Tính Năng Nâng Cao Dự Kiến (Future Pipeline)

### 3.1. Rate Limiting theo Tokens Per Minute (TPM)
- Hiện tại hệ thống đã có Rate Limit theo Requests Per Minute (RPM) và Budget Quota theo USD.
- Bổ sung thêm Token Bucket đếm số lượng Token đầu vào/ra theo phút (TPM) để tránh tình trạng một vài request khổng lồ chiếm trọn băng thông của model upstream.

### 3.2. Semantic Caching với Vector Embeddings
- Nâng cấp bộ đệm Redis Response Cache hiện tại (dựa trên SHA-256 hash của prompt):
- Tích hợp mô hình embedding cục bộ hoặc API embedding siêu nhẹ để so sánh độ tương đồng ngữ nghĩa (Cosine Similarity $\ge 0.95$).
- Giúp trả lời tức thì các câu hỏi trùng ý nghĩa (ví dụ: *"Thủ đô của Pháp là gì?"* và *"Pháp có thủ đô là thành phố nào?"*), tiết kiệm đến 80% chi phí và giảm độ trễ xuống dưới 10ms.

### 3.3. Request Hedging / Speculative Routing
- Với các request VIP đòi hỏi độ trễ cực thấp: Gửi đồng thời request tới 2 nhà cung cấp (ví dụ Azure OpenAI và OpenAI Direct). Gateway giữ luồng của bên nào trả về chunk đầu tiên (TTFT) sớm hơn và hủy kết nối với bên còn lại.

---

## 📅 Lộ Trình Triển Khai Đề Xuất (Release Plan)

| Phiên bản | Mục tiêu chính | Trạng thái |
| :--- | :--- | :---: |
| **v0.2.x** | Distributed architecture, AWS IAM, Multi-tenancy, Alerting, i18n, Open-Source Showcase | **Đã hoàn thành** |
| **v0.3.0** | **Virtual Models / Model Aliasing** + Fallback Chains cơ bản | *Kế hoạch kế tiếp* |
| **v0.3.1** | **Tối ưu hóa CLI `llmgw`** (Profile, Alias CLI, Live Tail Logs) | *Kế hoạch* |
| **v0.4.0** | Traffic Splitting (Canary A/B) + TPM Rate Limiting | *Kế hoạch* |
| **v0.5.0** | Semantic Vector Caching & Predictive Circuit Breaker | *Nghiên cứu* |
| **v1.0.0** | Enterprise Production Ready (Zero-Knowledge High Concurrency) | *Mục tiêu dài hạn* |
