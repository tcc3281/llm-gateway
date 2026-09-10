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

## 📌 4. Tính Năng Trọng Tâm 4: Cách Ly Vật Lý & Shard Riêng Biệt (Cell-Based Isolation & Dedicated Shards)

### 🎯 Bài toán thực tế: Vấn Nạn "Noisy Neighbor"
Phần lớn gateway mã nguồn mở hiện nay chỉ phân quyền logic trên cùng một cụm hạ tầng dùng chung. Điều này tạo ra rủi ro nghiêm trọng:
- Một tenant bị nghẽn (dính lỗi 429 từ nhà cung cấp, chạy agent lặp vô tận, hoặc spam tải) có thể kéo tụt hiệu năng hoặc làm sập toàn bộ hệ thống.
- Các khách hàng khối Tài chính (FinTech), Y tế, Thương mại điện tử yêu cầu cam kết SLA nghiêm ngặt và không chấp nhận dùng chung luồng mạng/bộ nhớ với tenant khác.

### 💡 Kiến Trúc Phân Mảnh (Cell-Based Isolation) & Chiến Lược Thương Mại
Khả năng cách ly hạ tầng vật lý theo Tenant là "mỏ vàng" để thương mại hóa sản phẩm cấp doanh nghiệp:

```
                          [ LLM Gateway Traffic Router ]
                                         │
                 ┌───────────────────────┴───────────────────────┐
                 ▼                                               ▼
     [ Shared Node Pool ]                           [ Dedicated Shard Pool ]
   (Free / Starter / Dev)                       (Enterprise / VIP Tenants)
   - Chịu giới hạn luồng chung                  - Node proxy & tài nguyên riêng biệt
   - Dùng chung hạn mức API Key                 - Gán API Key Tier cao cấp (PTU/Tier 5)
   - Tự động chuyển vùng suy thoái             - Cam kết 100% SLA & Zero Noisy Neighbor
```

### 🚀 Các trụ cột năng lực cốt lõi:
1. **Bán Gói "Dedicated Shard" (Mô hình kiếm tiền trực tiếp)**:
   - Doanh nghiệp lớn trả phí hàng tháng để sở hữu một hoặc nhiều node proxy riêng biệt. Lưu lượng đi trên kênh xử lý độc lập, được gán riêng API Key cấp cao phía upstream.
2. **Blast Radius Containment (Khoanh vùng sự cố)**:
   - Sự cố vòng lặp của một AI Agent hay script thử nghiệm của dev chỉ gây ảnh hưởng cục bộ trên shard đó. Cổng AI phục vụ khách hàng chính vẫn hoạt động ổn định 100%.
3. **Guaranteed Concurrency (Cam kết luồng đồng thời)**:
   - Phân bổ tài nguyên chính xác: Shard nội bộ tối đa 20 luồng, Shard ứng dụng core luôn được bảo đảm 500 luồng trực tiếp không bị tranh chấp qua cơ chế Concurrency Semaphore trên RAM/Redis.
4. **Tối ưu hóa hạ tầng GPU Hybrid (Smart Cost Arbitrage & Smart Spilling)**:
   - **Internal Node Group**: Toàn bộ request từ IDE lập trình viên và tác vụ thử nghiệm chuyển thẳng về cụm GPU on-premise (vLLM / SGLang / Ollama) $\rightarrow$ Chi phí cận biên bằng 0.
   - **Cơ chế Smart Spilling**: Gateway chủ động thăm dò endpoint metrics thời gian thực từ vLLM (`vllm:gpu_cache_usage_factor`). Khi GPU cache $> 85\%$ hoặc nghẽn hàng đợi, Gateway tự động tràn (spill) request sang Cloud API (OpenAI / Anthropic) tức thì trong suốt với người dùng.
5. **Cơ chế "Dynamic Quarantine" (Tự động chuyển vùng suy thoái)**:
   - Khi phát hiện một tenant tăng đột biến bất thường, tỷ lệ lỗi $> 40\%$ hoặc latency $> 10\text{s}$, Gateway không ngắt kết nối thô bạo (429) mà tự động gắn cờ cách ly, chuyển hướng các request tiếp theo sang **Degraded Pool**. Tenant vi phạm vẫn được phục vụ với tốc độ vừa phải, còn các tenant chuẩn hoàn toàn không bị ảnh hưởng độ trễ.

### 🛠️ Kiến Trúc Triển Khai Kép (Dual-Tier Architecture: App & Infrastructure)

Để đáp ứng linh hoạt từ Startup nhỏ đến Ngân hàng/Tập đoàn lớn, hệ thống phân định rạch ròi giữa **Logic Ứng Dụng (Go Code)** và **Đóng Gói Hạ Tầng (Docker / K8s)**:

```
               [ Khách hàng / Client Request ]
                              │
           ┌──────────────────┴──────────────────┐
           ▼                                     ▼
   【 Lựa chọn 1: Docker Compose 】       【 Lựa chọn 2: Kubernetes (K8s) 】
   (Dành cho On-Premise / VPS)           (Dành cho Doanh nghiệp lớn)
           │                                     │
    [ Nginx Reverse Proxy ]                [ K8s Ingress Controller ]
    (Routing theo Header/API Key)         (Routing theo Subdomain / Header)
           │                                     │
     ┌─────┴─────┐                         ┌─────┴─────┐
     ▼           ▼                         ▼           ▼
[Container: [Container:               [Pod: Shared] [Pod: VIP Enterprise]
  Shared]     VIP Shard]              (Standard Nodes) (Pinned GPU Nodes)
     │           │                         │           │
     └─────┬─────┘                         └─────┬─────┘
           │                                     │
           └──────────────────┬──────────────────┘
                              ▼
           【 Phần App: LLM Gateway (Go Core) 】
           - Nhận biết biến môi trường SHARD_TIER (shared, enterprise-vip)
           - Giới hạn Concurrency Semaphore theo Tenant
           - Tự đo metrics vLLM để Smart Spilling
           - Dynamic Quarantine cách ly tenant spam
```

#### 1. Tầng Ứng Dụng (Application Layer - Code Go):
- Giữ bản chất **Stateless** và điều khiển qua Biến Môi Trường (Environment Variables):
  - `SHARD_TIER=shared`: Áp dụng giới hạn concurrency nghiêm ngặt, chia sẻ hạn mức chung.
  - `SHARD_TIER=enterprise`: Tối ưu concurrency cao, kết nối upstream API Key cấp cao (Azure PTU / Tier 5).
  - `VLLM_METRICS_URL`: Endpoint giám sát tải VRAM GPU on-premise cho thuật toán Smart Spilling.

#### 2. Tầng Hạ Tầng (Infrastructure Layer - Đóng gói linh hoạt):
Cung cấp sẵn 2 bộ cấu hình đóng gói chuẩn trong thư mục `deploy/`:
* **📦 Gói 1: Docker Compose Sharded (`deploy/docker-compose/docker-compose.sharded.yml`):**
  - Dành cho Startup, SME, chạy thử nghiệm (PoC), hoặc server đơn lẻ có gắn GPU on-premise.
  - Khởi chạy 2 cụm container (`gateway-shared` và `gateway-vip`) kèm Nginx Reverse Proxy đứng trước để phân luồng theo API Key prefix (`sk-vip-...` hoặc header `X-Tenant-Tier`).
* **☸️ Gói 2: Kubernetes Manifests & Helm Chart (`deploy/kubernetes/helm/`):**
  - Dành cho Enterprise, Cloud-Native (AWS EKS, GCP GKE, OpenShift).
  - Tích hợp **Node Affinity / GPU Pinning** (`nodeSelector: tier=enterprise`) để ghim Pod VIP trên phần cứng riêng biệt.
  - Tích hợp K8s Ingress Controller và cấu hình HPA (Horizontal Pod Autoscaler) tự động co giãn theo số lượng kết nối đồng thời.

---

## 📌 5. Tính Năng Trọng Tâm 5: Scale Cụm Tự Thân Thông Minh (Cluster-Native Scaling & Gossip Mesh)

### 🎯 Điểm nghẽn của các giải pháp hiện tại
- **Phụ thuộc Redis/DB trung tâm**: Khi cụm proxy scale từ 5 lên 50 pod, tất cả pod đều spam query kiểm tra token/quota vào một cụm Redis duy nhất $\rightarrow$ Redis trở thành điểm nghẽn cổ chai (SPOF) và lock contention.
- **Bất cập của Kubernetes HPA thông thường**: HPA scale theo CPU. Nhưng LLM Streaming giữ kết nối HTTP SSE rất lâu (10s – 60s) trong khi CPU tiêu thụ rất ít $\rightarrow$ K8s không chịu scale thêm pod, dẫn đến cạn kiệt TCP connection pool.

### 💡 Giải Pháp Cluster-Native bằng Go:
1. **Cụm Mesh gọn nhẹ bằng Gossip Protocol (`hashicorp/memberlist`)**:
   - Các node Gateway viết bằng Go tự tìm thấy nhau trong mạng nội bộ, tự phát hiện khi có node mới bật lên hoặc node cũ chết mà không cần cài đặt dịch vụ Service Discovery phức tạp.
2. **Local Quota Token-Leasing (Thuật toán mượn hạn mức)**:
   - Thay vì mỗi request phải ping Redis để kiểm tra budget: Mỗi node proxy "mượn" trước một lượng budget nhỏ (ví dụ 1$ hoặc 50.000 tokens) từ Control Plane lưu trong RAM local.
   - Node tự trừ trực tiếp trên RAM. Khi xài gần hết mới xin cấp thêm bất đồng bộ $\rightarrow$ Cụm proxy scale lên hàng trăm node mà database trung tâm vẫn hoàn toàn rảnh rỗi.
3. **Cluster-Aware KV-Cache Routing & Consistent Hashing**:
   - Khi đứng trước cụm nhiều máy chủ GPU nội bộ (vLLM / SGLang), proxy dùng thuật toán băm nhất quán để đảm bảo cùng một `session_id` hoặc `tenant_id` sẽ luôn điều phối về cùng một GPU worker $\rightarrow$ **Tránh tối đa KV-cache miss**, giúp GPU không phải tính toán pre-fill lại từ đầu.

---

## 📌 6. Tính Năng Trọng Tâm 6: Đăng Nhập Một Lần Doanh Nghiệp (Enterprise SSO & Identity Federation)

### 🎯 Mục tiêu
Loại bỏ việc quản lý username/password thủ công nội bộ, đáp ứng tiêu chuẩn kiểm toán bảo mật khắt khe của doanh nghiệp thông qua cơ chế **Single Sign-On (SSO)** với các Identity Provider (IdP) hàng đầu.

```text
[ Người Dùng / Kỹ Sư ] 
      │
      ├── 1. Truy cập ─────────► [ LLM Gateway Dashboard / CLI ]
      │                                       │
      │   2. Chuyển hướng xác thực            │
      ▼                                       ▼
[ Identity Provider (IdP) ] ◄─────────────────┘
(Google Workspace, Okta, Azure AD, Keycloak)
      │
      ├── 3. Xác thực người dùng (MFA/2FA)
      │
      └── 4. Cấp OIDC JWT Token ──► Trả về LLM Gateway ──► Đăng nhập thành công & Map RBAC
```

### 💡 Các năng lực triển khai:
1. **Hỗ trợ Đa Giao thức Chuẩn Doanh Nghiệp**:
   - **OIDC (OpenID Connect)**: Tích hợp Google Workspace, GitHub Enterprise, Okta, Keycloak qua JSON Web Tokens (JWT).
   - **SAML 2.0**: Tương thích hệ thống phân quyền của ngân hàng và tập đoàn lớn (Microsoft Entra ID / Azure AD, Ping Identity).
2. **Xác thực CLI Thông Minh (`llmgw login`)**:
   - Kỹ sư chạy lệnh `llmgw login` trên terminal: CLI tự động mở trình duyệt web điều hướng tới IdP của công ty.
   - Sau khi đăng nhập và vượt qua MFA, IdP trả token tạm thời về CLI. Kỹ sư không bao giờ phải gõ hay lưu mật khẩu trên máy cá nhân.
3. **Đồng bộ Nhóm & Phân Quyền Tự Động (Group-to-Role Mapping & Instant Offboarding)**:
   - Nhóm người dùng từ Okta/Azure AD (ví dụ: `DataScience-Team`, `AI-Engineers`) tự động được map sang Role và Tenant tương ứng trên Gateway.
   - Khi nhân viên nghỉ việc: Khóa tài khoản tại IdP trung tâm sẽ ngay lập tức vô hiệu hóa quyền truy cập trên cả Web Dashboard lẫn CLI Gateway.

---

## 📅 Lộ Trình Triển Khai Đề Xuất (Release Plan)

| Phiên bản | Mục tiêu chiến lược | Các tính năng trọng tâm | Trạng thái |
| :--- | :--- | :--- | :---: |
| **v0.2.x** | Enterprise Foundation | Distributed Architecture, AWS IAM, Multi-tenancy, Alerting, i18n, MIT License | **Đã hoàn thành** |
| **v0.3.x** | **Virtual Models & Plugins** | Virtual Model Aliasing, Fallback Chains, 10 Modular Plugins, Scope Bindings | **Đã hoàn thành (v0.3.1)** |
| **v0.4.0** | **CLI Tooling & Live Logs** | Quản lý Profile đa môi trường, `llmgw plugins` CLI, Live Tail Logs | *Kế hoạch kế tiếp* |
| **v0.5.0** | **Advanced Intelligence Plugins** | Plugins: `semantic-cache` (Vector Embedding), `pii-masking`, `prompt-guard`, `langfuse-exporter` | *Kế hoạch* |
| **v0.6.0** | **Cell-Based Isolation & Sharding** | Dedicated Shard Pool (Docker Compose + K8s Helm), Dynamic Quarantine, Smart GPU Spilling | *Kế hoạch* |
| **v0.7.0** | **Cluster-Native Mesh Scaling** | Gossip Protocol Mesh (`memberlist`), Local Quota Token-Leasing, Cluster-Aware KV-Cache Routing | *Nghiên cứu* |
| **v1.0.0** | **Enterprise Production Ready** | Enterprise SSO (SAML 2.0 / OIDC), Custom WASM/Go Plugin SDK, Zero-Knowledge Engine | *Mục tiêu dài hạn* |

