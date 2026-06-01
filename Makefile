# Dia — developer task runner
.DEFAULT_GOAL := help
SHELL := /bin/bash

GO        ?= go
BIN_DIR   := bin

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z0-9_.-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

## ── Infra ────────────────────────────────────────────────────
.PHONY: infra-up
infra-up: ## Start Postgres + Redis + NATS (dev)
	docker compose up -d

.PHONY: infra-down
infra-down: ## Stop dev infra
	docker compose down

.PHONY: infra-reset
infra-reset: ## Stop dev infra and wipe volumes
	docker compose down -v

## ── Go services ──────────────────────────────────────────────
.PHONY: tidy
tidy: ## go mod tidy
	$(GO) mod tidy

.PHONY: build
build: ## Build the Go binaries into ./bin
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/worker ./cmd/worker
	$(GO) build -o $(BIN_DIR)/api    ./cmd/api

.PHONY: worker
worker: ## Run the worker (bot brain)
	$(GO) run ./cmd/worker

.PHONY: api
api: ## Run the API (dashboard backend)
	$(GO) run ./cmd/api

.PHONY: test
test: ## Run Go tests
	$(GO) test ./internal/... ./cmd/...

.PHONY: vet
vet: ## go vet
	$(GO) vet ./internal/... ./cmd/...

## ── Migrations ───────────────────────────────────────────────
.PHONY: migrate
migrate: ## Apply DB migrations (runs the api binary's --migrate path)
	$(GO) run ./cmd/api --migrate-only

## ── Gateway (Elixir) ─────────────────────────────────────────
.PHONY: gateway
gateway: ## Run the Elixir gateway
	cd gateway && mix run --no-halt

.PHONY: gateway-deps
gateway-deps: ## Fetch Elixir deps
	cd gateway && mix deps.get

## ── Web (SvelteKit) ──────────────────────────────────────────
.PHONY: web
web: ## Run the SvelteKit dev server
	cd web && pnpm dev

.PHONY: web-install
web-install: ## Install web deps
	cd web && pnpm install
