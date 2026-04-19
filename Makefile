SHELL := /bin/bash
COMPOSE := docker compose

.PHONY: help setup setup-mobile up down restart logs dev dev-db dev-mobile dev-web dev-all build test lint fmt tidy typecheck migrate migrate-down migrate-status migrate-create db-shell db-reset seed token clean

help: ## Show available commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

setup: setup-mobile ## Bootstrap .env files and install mobile deps
	@if [ ! -f backend/.env ]; then cp .env.example backend/.env; fi
	@echo "Setup complete. Edit backend/.env (set ANTHROPIC_API_KEY) then run 'make dev' or 'make up'."

setup-mobile: ## Install Expo mobile deps and copy .env.local
	@if [ ! -f mobile/.env.local ]; then cp mobile/.env.example mobile/.env.local; fi
	@npm install --prefix mobile --silent

up: ## Run full stack in Docker (timescaledb + migrate + api)
	$(COMPOSE) up --build -d

down: ## Stop Docker stack
	$(COMPOSE) down

restart: ## Restart Docker stack
	$(MAKE) down && $(MAKE) up

logs: ## Tail Docker logs
	$(COMPOSE) logs -f

dev-db: ## Start timescaledb + migrations only
	$(COMPOSE) up -d timescaledb
	$(COMPOSE) up migrate

dev: dev-db ## Run API on host against dockerized timescaledb
	@set -a; source backend/.env; set +a; \
		cd backend && DATABASE_URL=postgres://postgres:postgres@localhost:5434/agent_app?sslmode=disable go run ./cmd/server

dev-mobile: ## Run Expo dev server (iOS / Android)
	cd mobile && EXPO_PUBLIC_API_URL=http://localhost:8080 npm run start

dev-web: ## Run Expo Web (same codebase, browser)
	cd mobile && EXPO_PUBLIC_API_URL=http://localhost:8080 npm run web

dev-all: ## Run API + Expo Web together
	@trap 'kill 0' EXIT; \
		($(MAKE) dev 2>&1 | sed 's/^/[api] /') & \
		($(MAKE) dev-web 2>&1 | sed 's/^/[web] /') & \
		wait

build: ## Build backend binaries
	cd backend && go build ./...

test: ## Run backend tests
	cd backend && go test ./...

lint: ## Run go vet
	cd backend && go vet ./...

fmt: ## Format Go source
	cd backend && gofmt -w .

tidy: ## go mod tidy
	cd backend && go mod tidy

typecheck: ## Typecheck the Expo app
	cd mobile && npm run typecheck

migrate: ## Run migrations up (in Docker)
	$(COMPOSE) up migrate --no-deps

migrate-down: ## Roll back last migration
	$(COMPOSE) run --rm migrate /bin/migrate down

migrate-status: ## Show migration status
	$(COMPOSE) run --rm migrate /bin/migrate status

migrate-create: ## Create a new migration: make migrate-create NAME=add_widgets
	@if [ -z "$(NAME)" ]; then echo "usage: make migrate-create NAME=migration_name"; exit 1; fi
	@TS=$$(date +%Y%m%d%H%M%S); \
		FILE="backend/internal/db/migrations/$${TS}_$(NAME).sql"; \
		printf -- "-- +goose Up\n-- +goose StatementBegin\n\n-- +goose StatementEnd\n\n-- +goose Down\n-- +goose StatementBegin\n\n-- +goose StatementEnd\n" > "$$FILE"; \
		echo "created $$FILE"

db-shell: ## Open psql shell
	docker exec -it agent-app-timescaledb psql -U postgres -d agent_app

db-reset: ## Recreate local database
	$(COMPOSE) down -v
	$(COMPOSE) up -d timescaledb
	sleep 3
	$(COMPOSE) up migrate --no-deps

seed: ## Issue dev JWT for dev@local
	curl -s -X POST http://localhost:8080/auth/login -H "Content-Type: application/json" -d '{"email":"dev@local","name":"Dev User"}'

token: seed ## Alias for seed

clean: ## Remove generated artifacts
	$(COMPOSE) down -v
	rm -rf backend/bin mobile/node_modules mobile/.expo
