SHELL := /bin/sh

BACKEND    := backend
FRONTEND   := frontend
MIGRATIONS := $(BACKEND)/migrations
DB_URL     ?= postgres://voice:voice@localhost:5432/voice_tool?sslmode=disable

.DEFAULT_GOAL := help

.PHONY: help
help: ## Liệt kê các target
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

# ---------------------------------------------------------------------------
# Setup
# ---------------------------------------------------------------------------

.PHONY: setup
setup: ## Tạo .env từ .env.example và cài tool dev
	@test -f .env || cp .env.example .env
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.1
	cd $(FRONTEND) && npm install

# ---------------------------------------------------------------------------
# Infra
# ---------------------------------------------------------------------------

.PHONY: up
up: ## Chạy toàn bộ stack (postgres, redis, minio, api, worker, frontend)
	docker compose up -d --build

.PHONY: infra
infra: ## Chỉ chạy hạ tầng (postgres, redis, minio) để dev local
	docker compose up -d postgres redis minio

.PHONY: down
down: ## Dừng stack
	docker compose down

.PHONY: clean
clean: ## Dừng stack và xoá volume (mất dữ liệu)
	docker compose down -v

.PHONY: logs
logs: ## Xem log api + worker + scheduler
	docker compose logs -f api worker scheduler

# ---------------------------------------------------------------------------
# Database
# ---------------------------------------------------------------------------

.PHONY: migrate-up
migrate-up: ## Chạy migration
	migrate -path $(MIGRATIONS) -database "$(DB_URL)" up

.PHONY: migrate-down
migrate-down: ## Rollback 1 bước migration
	migrate -path $(MIGRATIONS) -database "$(DB_URL)" down 1

.PHONY: migrate-new
migrate-new: ## Tạo migration mới: make migrate-new name=add_xxx
	migrate create -ext sql -dir $(MIGRATIONS) -seq $(name)

.PHONY: sqlc
sqlc: ## Sinh lại code Go từ SQL
	cd $(BACKEND) && sqlc generate

.PHONY: gen-key
gen-key: ## Sinh TOKEN_ENCRYPTION_KEY mới
	cd $(BACKEND) && go run ./cmd/genkey

.PHONY: seed
dev-token: ## Phát JWT để gọi API khi dev: make dev-token user=<uuid>
	@cd backend && go run ./cmd/devtoken $(user) $(or $(role),admin)

seed: ## Nạp dữ liệu mẫu (prompt, ai_engine)
	psql "$(DB_URL)" -f $(BACKEND)/seeds/seed.sql

# ---------------------------------------------------------------------------
# Backend
# ---------------------------------------------------------------------------

.PHONY: api
api: ## Chạy API local
	cd $(BACKEND) && go run ./cmd/api

.PHONY: worker
worker: ## Chạy worker local (chạy được nhiều process song song)
	cd $(BACKEND) && go run ./cmd/worker

.PHONY: scheduler
scheduler: ## Chạy scheduler local (CHỈ 1 process)
	cd $(BACKEND) && go run ./cmd/scheduler

.PHONY: build
build: ## Build 3 binary
	cd $(BACKEND) && go build -o ../bin/api ./cmd/api && go build -o ../bin/worker ./cmd/worker && go build -o ../bin/scheduler ./cmd/scheduler

.PHONY: test
test: ## Chạy test backend
	cd $(BACKEND) && go test ./...

.PHONY: lint
lint: ## gofmt + go vet
	cd $(BACKEND) && gofmt -l . && go vet ./...

# ---------------------------------------------------------------------------
# Frontend
# ---------------------------------------------------------------------------

.PHONY: web
web: ## Chạy frontend dev server
	cd $(FRONTEND) && npm run dev

.PHONY: web-build
web-build: ## Build frontend
	cd $(FRONTEND) && npm run build

.PHONY: web-lint
web-lint: ## Lint frontend
	cd $(FRONTEND) && npm run lint
