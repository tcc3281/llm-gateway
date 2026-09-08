# ==============================================================================
# LLM Gateway - Makefile
# ==============================================================================

.PHONY: help \
        up down restart build logs logs-all ps health up-monitoring up-all \
        test-backend lint-backend lint-cli secret-scan check-all \
        web-install web-dev web-build \
        user-list user-create key-list key-create key-revoke \
        provider-list provider-set model-list model-add model-delete model-discover stats \
        clean

# ------------------------------------------------------------------------------
# Variables & Configurations
# ------------------------------------------------------------------------------
DOCKER_COMPOSE ?= docker compose
CLI            ?= uv run --project cli llmgw
BUDGET         ?= 50.0
RPM            ?= 100
INTERVAL       ?= hour

# ------------------------------------------------------------------------------
# Help / Menu
# ------------------------------------------------------------------------------
help:
	@echo "========================================================================"
	@echo "                   🚀 LLM Gateway Management Tasks                      "
	@echo "========================================================================"
	@echo ""
	@echo "🐳 [Docker & Services]"
	@echo "  make up               - Khởi động core stack ở chế độ background"
	@echo "  make down             - Dừng toàn bộ containers"
	@echo "  make restart          - Khởi động lại hệ thống (down + up)"
	@echo "  make build            - Build lại Docker image và khởi động stack"
	@echo "  make logs             - Theo dõi logs của gateway container"
	@echo "  make logs-all         - Theo dõi logs toàn bộ services trong stack"
	@echo "  make ps               - Trạng thái các container đang chạy"
	@echo "  make health           - Kiểm tra health check admin server (:8081)"
	@echo "  make up-monitoring    - Khởi động core stack kèm Prometheus & Grafana"
	@echo "  make up-all           - Khởi động tất cả profiles service"
	@echo ""
	@echo "🛡️  [Quality, Testing & Security]"
	@echo "  make secret-scan      - Quét secret bằng Gitleaks (tự động dùng Docker)"
	@echo "  make test-backend     - Chạy Go unit tests với race detector & coverage"
	@echo "  make lint-backend     - Kiểm tra Go vet và go.mod"
	@echo "  make lint-cli         - Kiểm tra và xác thực CLI entrypoint"
	@echo "  make check-all        - Chạy toàn bộ test, lint và secret scanning"
	@echo ""
	@echo "💻 [Web Dashboard (web/)]"
	@echo "  make web-install      - Cài đặt dependencies (npm install)"
	@echo "  make web-dev          - Chạy Vite development server (:5173)"
	@echo "  make web-build        - Build production bundle vào internal/web/dist"
	@echo ""
	@echo "🔑 [CLI: Users & API Keys]"
	@echo "  make user-list        - Danh sách tất cả người dùng"
	@echo "  make user-create      - Tạo user mới (Cần: EMAIL, NAME)"
	@echo "  make key-list         - Danh sách tất cả API keys của gateway"
	@echo "  make key-create       - Tạo key mới (Cần: UID, NAME | Tùy chọn: BUDGET, RPM)"
	@echo "  make key-revoke       - Thu hồi/vô hiệu hóa API key (Cần: KID)"
	@echo ""
	@echo "🤖 [CLI: Providers & Models]"
	@echo "  make provider-list    - Danh sách API key nhà cung cấp upstream"
	@echo "  make provider-set     - Cấu hình provider key (Cần: PVD, KEY)"
	@echo "  make model-list       - Danh sách models và bảng giá định tuyến"
	@echo "  make model-add        - Thêm model và giá (Cần: PVD, MODEL, PROMPT, COMP)"
	@echo "  make model-delete     - Xóa cấu hình model (Cần: MID)"
	@echo "  make model-discover   - Tự động discover models từ các providers"
	@echo ""
	@echo "📊 [CLI: Analytics & Utilities]"
	@echo "  make stats            - Xem thống kê chi tiêu (Tùy chọn: INTERVAL, UID)"
	@echo "  make clean            - Dọn dẹp cache, file build và test coverage"
	@echo ""
	@echo "========================================================================"

# ==============================================================================
# 1. Docker & Services
# ==============================================================================
up:
	$(DOCKER_COMPOSE) up -d

down:
	$(DOCKER_COMPOSE) down

restart: down up

build:
	$(DOCKER_COMPOSE) up -d --build

logs:
	$(DOCKER_COMPOSE) logs -f gateway

logs-all:
	$(DOCKER_COMPOSE) logs -f

ps:
	$(DOCKER_COMPOSE) ps

health:
	@curl -s http://localhost:8081/health || echo "❌ Admin server chưa sẵn sàng."

up-monitoring:
	$(DOCKER_COMPOSE) --profile monitoring up -d

up-all:
	$(DOCKER_COMPOSE) --profile all up -d

# ==============================================================================
# 2. Quality, Testing & Security
# ==============================================================================
secret-scan:
	@echo "🔍 Đang quét rò rỉ secret / API key bằng Gitleaks..."
	@if command -v gitleaks > /dev/null 2>&1; then \
		gitleaks detect -v --source . --config .gitleaks.toml; \
	else \
		docker run --rm -v "$$(pwd):/path" zricethezav/gitleaks:latest detect -v --source=/path --config=/path/.gitleaks.toml; \
	fi

test-backend:
	go test -v -race -timeout 10m -covermode=atomic -coverprofile=coverage.out ./...

lint-backend:
	go vet ./...

lint-cli:
	$(CLI) --help > /dev/null && echo "✅ CLI entrypoint hợp lệ."

check-all: lint-backend test-backend lint-cli secret-scan
	@echo "✅ Toàn bộ bài kiểm tra (Lint, Test, Secret Scan) đã hoàn tất thành công!"

# ==============================================================================
# 3. Web Dashboard (web/)
# ==============================================================================
web-install:
	cd web && npm install

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build

# ==============================================================================
# 4. CLI: User Management
# ==============================================================================
user-list:
	$(CLI) user list

user-create:
	@if [ -z "$(EMAIL)" ] || [ -z "$(NAME)" ]; then \
		echo "❌ Lỗi: Thiếu tham số EMAIL hoặc NAME."; \
		echo "👉 Ví dụ: make user-create EMAIL=dev@abc.com NAME=\"Dev 1\""; \
		exit 1; \
	fi
	$(CLI) user create --email "$(EMAIL)" --name "$(NAME)" --role user

# ==============================================================================
# 5. CLI: API Key Management
# ==============================================================================
key-list:
	$(CLI) key list

key-create:
	@if [ -z "$(UID)" ] || [ -z "$(NAME)" ]; then \
		echo "❌ Lỗi: Thiếu tham số UID hoặc NAME."; \
		echo "👉 Ví dụ: make key-create UID=<uuid> NAME=\"App Key\" BUDGET=50 RPM=100"; \
		exit 1; \
	fi
	$(CLI) key create --user-id "$(UID)" --name "$(NAME)" --budget "$(BUDGET)" --rpm "$(RPM)"

key-revoke:
	@if [ -z "$(KID)" ]; then \
		echo "❌ Lỗi: Thiếu tham số KID."; \
		echo "👉 Ví dụ: make key-revoke KID=<uuid>"; \
		exit 1; \
	fi
	$(CLI) key revoke --id "$(KID)"

# ==============================================================================
# 6. CLI: Provider & Model Management
# ==============================================================================
provider-list:
	$(CLI) provider list

provider-set:
	@if [ -z "$(PVD)" ] || [ -z "$(KEY)" ]; then \
		echo "❌ Lỗi: Thiếu tham số PVD (provider) hoặc KEY."; \
		echo "👉 Ví dụ: make provider-set PVD=openai KEY=sk-..."; \
		exit 1; \
	fi
	$(CLI) provider set --provider "$(PVD)" --api-key "$(KEY)"

model-list:
	$(CLI) model list

model-add:
	@if [ -z "$(PVD)" ] || [ -z "$(MODEL)" ] || [ -z "$(PROMPT)" ] || [ -z "$(COMP)" ]; then \
		echo "❌ Lỗi: Thiếu tham số PVD, MODEL, PROMPT, COMP."; \
		echo "👉 Ví dụ: make model-add PVD=openai MODEL=gpt-4o PROMPT=2.5 COMP=10"; \
		exit 1; \
	fi
	$(CLI) model add --provider "$(PVD)" --name "$(MODEL)" --prompt-price "$(PROMPT)" --completion-price "$(COMP)"

model-delete:
	@if [ -z "$(MID)" ]; then \
		echo "❌ Lỗi: Thiếu tham số MID."; \
		echo "👉 Ví dụ: make model-delete MID=<uuid>"; \
		exit 1; \
	fi
	$(CLI) model delete --id "$(MID)"

model-discover:
	$(CLI) model discover

# ==============================================================================
# 7. CLI: Analytics & Utilities
# ==============================================================================
stats:
	@if [ -z "$(UID)" ]; then \
		$(CLI) stats --interval "$(INTERVAL)"; \
	else \
		$(CLI) stats --interval "$(INTERVAL)" --user-id "$(UID)"; \
	fi

clean:
	@echo "🧹 Dọn dẹp build artifacts và coverage..."
	@rm -f coverage.out profile.cov
	@rm -rf internal/web/dist/*
	@touch internal/web/dist/index.html
	@echo "✨ Hoàn tất dọn dẹp."
