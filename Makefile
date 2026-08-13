.PHONY: help up down build restart logs health web-install web-dev web-build user-list user-create key-list key-create key-revoke provider-list provider-set model-list model-add model-delete stats

CONTAINER_NAME = llm_gateway
BUDGET ?= 50.0
RPM ?= 100
INTERVAL ?= hour

help:
	@echo "========================================="
	@echo "    LLM Gateway Makefile Commands"
	@echo "========================================="
	@echo ""
	@echo "--- Docker & System ---"
	@echo "make up             - Khởi động hệ thống (docker-compose up -d)"
	@echo "make down           - Dừng hệ thống"
	@echo "make build          - Build lại và khởi động"
	@echo "make logs           - Xem logs của gateway container"
	@echo "make health         - Kiểm tra health server admin (port 8081)"
	@echo ""
	@echo "--- Dashboard Frontend (web/) ---"
	@echo "make web-install    - Cài dependencies (npm install) trong web/"
	@echo "make web-dev        - Chạy Vite dev server (proxy tới admin :8081)"
	@echo "make web-build      - Build frontend vào internal/web/dist"
	@echo ""
	@echo "--- CLI: Users ---"
	@echo "make user-list      - Liệt kê tất cả người dùng"
	@echo "make user-create    - Tạo người dùng mới"
	@echo "                      (Cần: EMAIL, NAME) | Ví dụ: make user-create EMAIL=dev@abc.com NAME=\"Dev 1\""
	@echo ""
	@echo "--- CLI: API Keys ---"
	@echo "make key-list       - Liệt kê tất cả API keys"
	@echo "make key-create     - Tạo API key (Mặc định: BUDGET=$(BUDGET), RPM=$(RPM))"
	@echo "                      (Cần: UID, NAME)   | Ví dụ: make key-create UID=<uuid> NAME=\"App Key\""
	@echo "make key-revoke     - Thu hồi vô hiệu hoá API key"
	@echo "                      (Cần: KID)         | Ví dụ: make key-revoke KID=<uuid>"
	@echo ""
	@echo "--- CLI: Providers ---"
	@echo "make provider-list  - Liệt kê các API key của nhà cung cấp"
	@echo "make provider-set   - Thêm/cập nhật provider key"
	@echo "                      (Cần: PVD, KEY)    | Ví dụ: make provider-set PVD=openai KEY=sk-..."
	@echo ""
	@echo "--- CLI: Models & Pricing ---"
	@echo "make model-list     - Liệt kê models và bảng giá định tuyến"
	@echo "make model-add      - Thêm/cập nhật model"
	@echo "                      (Cần: PVD, MODEL, PROMPT, COMP) | Ví dụ: make model-add PVD=openai MODEL=gpt-4o PROMPT=2.5 COMP=10"
	@echo "make model-delete   - Xóa model"
	@echo "                      (Cần: MID)         | Ví dụ: make model-delete MID=<uuid>"
	@echo ""
	@echo "--- CLI: Analytics & Stats ---"
	@echo "make stats          - Xem thống kê (Mặc định: INTERVAL=$(INTERVAL))"
	@echo "                      (Tùy chọn: INTERVAL, UID) | Ví dụ: make stats INTERVAL=day"
	@echo ""

# Docker Commands
up:
	docker-compose up -d

down:
	docker-compose down

build: web-build
	docker-compose up -d --build

restart: down up

logs:
	docker-compose logs -f gateway

health:
	curl -s http://localhost:8081/health

# Users
user-list:
	uv run --project cli llmgw user list

user-create:
	@if [ -z "$(EMAIL)" ] || [ -z "$(NAME)" ]; then echo "❌ Lỗi: Thiếu tham số EMAIL hoặc NAME.\nVí dụ: make user-create EMAIL=dev@abc.com NAME=\"Dev\""; exit 1; fi
	uv run --project cli llmgw user create --email "$(EMAIL)" --name "$(NAME)" --role user

# Keys
key-list:
	uv run --project cli llmgw key list

key-create:
	@if [ -z "$(UID)" ] || [ -z "$(NAME)" ]; then echo "❌ Lỗi: Thiếu tham số UID hoặc NAME.\nVí dụ: make key-create UID=<uuid> NAME=\"App\""; exit 1; fi
	uv run --project cli llmgw key create --user-id "$(UID)" --name "$(NAME)" --budget "$(BUDGET)" --rpm "$(RPM)"

key-revoke:
	@if [ -z "$(KID)" ]; then echo "❌ Lỗi: Thiếu tham số KID.\nVí dụ: make key-revoke KID=<uuid>"; exit 1; fi
	uv run --project cli llmgw key revoke --id "$(KID)"

# Providers
provider-list:
	uv run --project cli llmgw provider list

provider-set:
	@if [ -z "$(PVD)" ] || [ -z "$(KEY)" ]; then echo "❌ Lỗi: Thiếu tham số PVD (provider) hoặc KEY.\nVí dụ: make provider-set PVD=openai KEY=sk-..."; exit 1; fi
	uv run --project cli llmgw provider set --provider "$(PVD)" --api-key "$(KEY)"

# Models
model-list:
	uv run --project cli llmgw model list

model-add:
	@if [ -z "$(PVD)" ] || [ -z "$(MODEL)" ] || [ -z "$(PROMPT)" ] || [ -z "$(COMP)" ]; then echo "❌ Lỗi: Thiếu tham số PVD, MODEL, PROMPT, COMP.\nVí dụ: make model-add PVD=openai MODEL=gpt-4o PROMPT=2.5 COMP=10"; exit 1; fi
	uv run --project cli llmgw model add --provider "$(PVD)" --name "$(MODEL)" --prompt-price "$(PROMPT)" --completion-price "$(COMP)"

model-delete:
	@if [ -z "$(MID)" ]; then echo "❌ Lỗi: Thiếu tham số MID.\nVí dụ: make model-delete MID=<uuid>"; exit 1; fi
	uv run --project cli llmgw model delete --id "$(MID)"

model-discover:
	uv run --project cli llmgw model discover

# Stats
stats:
	@if [ -z "$(UID)" ]; then \
		uv run --project cli llmgw stats --interval "$(INTERVAL)"; \
	else \
		uv run --project cli llmgw stats --interval "$(INTERVAL)" --user-id "$(UID)"; \
	fi

# Dashboard Frontend (web/)
web-install:
	cd web && npm install

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build
