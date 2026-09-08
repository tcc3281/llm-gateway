# Hướng Dẫn Đóng Góp (Contributing Guide)

Chào mừng bạn đến với dự án **LLM Gateway**! Chúng tôi trân trọng và hoan nghênh mọi sự đóng góp từ cộng đồng, từ việc sửa lỗi nhỏ, cải thiện tài liệu cho đến việc đề xuất và phát triển tính năng mới.

---

## 🧭 Mục lục
1. [Quy Tắc Chung (Code of Conduct)](#1-quy-tắc-chung-code-of-conduct)
2. [Quy Trình Phát Triển (Development Workflow)](#2-quy-trình-phát-triển-development-workflow)
3. [Thiết Lập Môi Trường (Local Environment Setup)](#3-thiết-lập-môi-trường-local-environment-setup)
4. [Quy Chuẩn Code & Linter (Coding Standards)](#4-quy-chuẩn-code--linter-coding-standards)
5. [Quy Chuẩn Commit (Commit Message Convention)](#5-quy-chuẩn-commit-commit-message-convention)
6. [Gửi Pull Request (Submitting Pull Requests)](#6-gửi-pull-request-submitting-pull-requests)

---

## 1. Quy Tắc Chung (Code of Conduct)
- Tôn trọng, hòa đồng và mang tính xây dựng trong mọi thảo luận và code review.
- Tập trung vào chất lượng code, độ ổn định hệ thống và trải nghiệm người dùng.
- Giữ vững nguyên tắc an toàn: Tuyệt đối không commit API keys, mật khẩu, file `.env` hoặc thông tin nhạy cảm vào repository.

---

## 2. Quy Trình Phát Triển (Development Workflow)
1. **Fork** repository về tài khoản cá nhân của bạn trên GitHub.
2. **Clone** repo đã fork về máy local:
   ```bash
   git clone https://github.com/<your-username>/llm-gateway.git
   cd llm-gateway
   ```
3. Tạo nhánh mới từ nhánh `master`:
   ```bash
   git checkout -b feat/ten-tinh-nang-moi master
   # hoặc:
   git checkout -b fix/ten-loi-can-sua master
   ```
4. Thực hiện các chỉnh sửa, kiểm tra kỹ lưỡng với Docker và Unit Tests.
5. Commit thay đổi và đẩy lên nhánh của bạn:
   ```bash
   git push -u origin feat/ten-tinh-nang-moi
   ```
6. Mở **Pull Request (PR)** vào nhánh `master` của repository gốc.

---

## 3. Thiết Lập Môi Trường (Local Environment Setup)

Dự án ưu tiên chạy và kiểm thử thông qua Docker để đảm bảo tính nhất quán trên mọi môi trường:

### A. Chạy toàn bộ hệ thống bằng Docker Compose (Khuyên dùng)
```bash
# Tạo file môi trường từ mẫu
cp .env.example .env

# Dựng và khởi chạy toàn bộ dịch vụ
docker compose up -d --build
```
- **Public Gateway**: `http://localhost:8080`
- **Admin Control Plane & Web UI**: `http://localhost:8081`

### B. Chạy chế độ kiến trúc phân tán (Distributed Mode)
```bash
docker compose -f docker-compose.distributed.yml up -d --build
```

### C. Phát triển Giao diện Web (Frontend Hot-Reload)
Giao diện quản trị nằm trong thư mục `web/` (React + Vite + TypeScript):
```bash
cd web
npm install
npm run dev
```
Trình duyệt mở tại `http://localhost:5173`.

---

## 4. Quy Chuẩn Code & Linter (Coding Standards)

- **Backend (Go 1.22+)**:
  - Tuân thủ chuẩn `gofmt` và `go vet`.
  - Quản lý dependencies qua Go Modules (`go mod tidy`).
  - Viết unit tests cho các hàm xử lý logic nghiệp vụ, adapter và permission evaluation (`go test -race ./...`).
- **Frontend (React 18 + TypeScript)**:
  - Tuân thủ TypeScript strict mode (`tsc -b`).
  - Toàn bộ text hiển thị trên UI **bắt buộc** phải được định nghĩa trong hệ thống đa ngôn ngữ `src/i18n/translations.ts` (cả Tiếng Việt `vi` và Tiếng Anh `en`), không hardcode text tiếng Anh hay tiếng Việt rời rạc.
  - Tái sử dụng các module/components, phân tách trách nhiệm rõ ràng (`components/`, `pages/`, `api/`).
- **CLI Tool (Python 3.11+)**:
  - Quản lý qua `uv` (`pyproject.toml`).
  - Đảm bảo lệnh `uv run --project cli llmgw --help` hoạt động không lỗi.

---

## 5. Quy Chuẩn Commit (Commit Message Convention)

Dự án áp dụng quy chuẩn **Conventional Commits**:

| Prefix | Ý nghĩa | Ví dụ |
| :--- | :--- | :--- |
| `feat:` | Tính năng mới | `feat(alert): support multi-channel webhook dispatching` |
| `fix:` | Sửa lỗi | `fix(iam): resolve CheckUserPermission method signature` |
| `docs:` | Cập nhật tài liệu | `docs: update deployment guide for distributed mode` |
| `refactor:` | Tái cấu trúc mã | `refactor(proxy): optimize SSE stream token forwarder` |
| `test:` | Thêm/sửa unit tests | `test(adapter): add claude 3.5 thinking test cases` |
| `chore:` | Bảo trì, cấu hình CI/CD | `chore(ci): upgrade buildx action to v3` |

---

## 6. Gửi Pull Request (Submitting Pull Requests)

Trước khi gửi PR, hãy tự kiểm tra danh sách sau:
- [ ] Code tuân thủ quy chuẩn định dạng (`gofmt`, strict TypeScript).
- [ ] Toàn bộ UI có hỗ trợ đầy đủ 2 ngôn ngữ qua `useLanguage()`.
- [ ] Toàn bộ các bước kiểm tra trong CI Pipeline (`backend-test`, `frontend-build`, `cli-lint`, `docker-build`) đều chạy xanh.
- [ ] Mô tả PR rõ ràng mục đích, phạm vi thay đổi và ảnh chụp màn hình (nếu có thay đổi UI).

Cảm ơn bạn đã đồng hành xây dựng **LLM Gateway**! 🎉
