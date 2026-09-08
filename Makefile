# ==============================================================================
# LLM Gateway - Makefile (Project Configuration & Execution)
# ==============================================================================

.PHONY: help env up down restart build logs logs-all ps health \
        up-monitoring up-all \
        web-install web-dev web-build \
        secret-scan test lint check clean

# ------------------------------------------------------------------------------
# Configurations
# ------------------------------------------------------------------------------
DOCKER_COMPOSE ?= docker compose

# ------------------------------------------------------------------------------
# Help / Menu
# ------------------------------------------------------------------------------
help:
	@echo "========================================================================"
	@echo "                   🚀 LLM Gateway Project Lifecycle                     "
	@echo "========================================================================"
	@echo ""
	@echo "⚙️  [Configuration]"
	@echo "  make env              - Tạo file .env từ .env.example (nếu chưa có)"
	@echo ""
	@echo "🐳 [Docker & Services]"
	@echo "  make up               - Khởi động core stack ở chế độ background"
	@echo "  make down             - Dừng toàn bộ containers"
	@echo "  make restart          - Khởi động lại hệ thống (down + up)"
	@echo "  make build            - Build lại Docker image và khởi động stack"
	@echo "  make logs             - Theo dõi logs của gateway container"
	@echo "  make logs-all         - Theo dõi logs toàn bộ services trong stack"
	@echo "  make ps               - Xem trạng thái các container"
	@echo "  make health           - Kiểm tra health check server admin (:8081)"
	@echo "  make up-monitoring    - Khởi động stack kèm Prometheus & Grafana"
	@echo "  make up-all           - Khởi động tất cả profiles service"
	@echo ""
	@echo "💻 [Web Dashboard (web/)]"
	@echo "  make web-install      - Cài đặt npm dependencies"
	@echo "  make web-dev          - Chạy Vite development server (:5173)"
	@echo "  make web-build        - Build production bundle vào internal/web/dist"
	@echo ""
	@echo "🛡️  [Quality & Security]"
	@echo "  make secret-scan      - Quét rò rỉ secret bằng Gitleaks (Docker/local)"
	@echo "  make test             - Chạy backend unit tests"
	@echo "  make lint             - Kiểm tra Go vet"
	@echo "  make check            - Chạy toàn bộ kiểm tra (lint + test + secret-scan)"
	@echo "  make clean            - Dọn dẹp cache, file build và coverage"
	@echo ""
	@echo "========================================================================"

# ==============================================================================
# 1. Configuration
# ==============================================================================
env:
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo "✅ Đã khởi tạo file .env từ .env.example. Vui lòng chỉnh sửa các key nếu cần."; \
	else \
		echo "ℹ️  File .env đã tồn tại, không cần tạo lại."; \
	fi

# ==============================================================================
# 2. Docker & Services
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
# 3. Web Dashboard (web/)
# ==============================================================================
web-install:
	cd web && npm install

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build

# ==============================================================================
# 4. Quality, Testing & Security
# ==============================================================================
secret-scan:
	@echo "🔍 Đang quét rò rỉ secret / API key bằng Gitleaks..."
	@if command -v gitleaks > /dev/null 2>&1; then \
		gitleaks detect -v --source . --config .gitleaks.toml; \
	else \
		docker run --rm -v "$$(pwd):/path" zricethezav/gitleaks:latest detect -v --source=/path --config=/path/.gitleaks.toml; \
	fi

test:
	go test -v -race -timeout 10m -covermode=atomic -coverprofile=coverage.out ./...

lint:
	go vet ./...

check: lint test secret-scan
	@echo "✅ Toàn bộ bài kiểm tra (Lint, Test, Secret Scan) đã hoàn tất thành công!"

clean:
	@echo "🧹 Dọn dẹp build artifacts và coverage..."
	@rm -f coverage.out profile.cov
	@rm -rf internal/web/dist/*
	@touch internal/web/dist/index.html
	@echo "✨ Hoàn tất dọn dẹp."
