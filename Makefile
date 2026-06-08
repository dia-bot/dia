# Dia — developer task runner.
#
# One compose file (docker-compose.yml); the targets below pick what runs.
#
#   make up      # the WHOLE dev stack in one shot: infra + migrations + seed + app
#   make down    # stop and remove it (volumes kept)
#
# Or drive the pieces yourself:
#   Docker:                make infra && make app && make seed
#   Native (host Go/Node):  make infra && make seed && make api / worker / web
#
# `make infra` brings up the shared stateful services (Postgres, Redis, NATS)
# under a pinned compose project so every git worktree targets the same stack.
# `make app` starts the application services (worker/api/web) against it. See
# `make help`.
.DEFAULT_GOAL := help
SHELL := /bin/bash

GO        ?= go
BIN_DIR   := bin

# Pin the compose project name so every worktree (root + the ones under
# ~/.agentd/worktrees) targets ONE shared infra stack instead of each directory
# spinning up its own Postgres/Redis/NATS. Set via `-p` (not
# COMPOSE_PROJECT_NAME) so fresh clones work with no environment setup.
COMPOSE  := docker compose -p dia-dev -f docker-compose.yml
# The app/gateway services sit behind compose profiles. Start commands name
# services explicitly (which starts them regardless of profile); project-wide
# commands (down/ps/logs) need the profiles enabled to see those services.
PROFILES := --profile app --profile gateway
# Full self-hostable stack lives in deploy/ and runs under its own project so it
# never collides with the dev infra above. `--env-file .env` is needed because
# the compose file lives in deploy/ — without it Compose looks for deploy/.env
# and silently ignores your repo-root .env. Added only when .env exists so
# `stack-down`/`stack-logs` still work before you've created one.
STACK_COMPOSE := docker compose -p dia-stack $(if $(wildcard .env),--env-file .env,) -f deploy/docker-compose.yml

# Seed / native targets talk to infra over the host-published Postgres port.
DATABASE_URL  ?= postgres://dia:dia@localhost:5432/dia?sslmode=disable
SEED_GUILD_ID ?=

# Infra service names (also the `make infra` up-list).
INFRA_SVCS := postgres redis nats

# App services started by `make app`. Override the whole set with
# `make app SVCS="api web"`, or add the gateway with `make app GATEWAY=1`.
APP_SVCS := worker api web
ifdef GATEWAY
APP_SVCS += gateway
endif
SVCS ?= $(APP_SVCS)

# `make up` seeds fixtures by default; SEED=0 skips them (migrations still run).
SEED ?= 1

# ── Expose the native dev servers off-box (Tailscale / LAN) ──────────────────
# Set PUBLIC_HOST to the address OTHER machines use to reach THIS one — your
# Tailscale IPv4 (`tailscale ip -4`) or MagicDNS name (`<host>.<tailnet>.ts.net`)
# — and pass it to the native targets:
#
#   make run PUBLIC_HOST=$(tailscale ip -4 | head -1)
#   make web PUBLIC_HOST=$(tailscale ip -4 | head -1)
#
# When set: the Vite dev server binds 0.0.0.0, the dashboard points its API/WS
# URLs at PUBLIC_HOST, and the API allows that origin (plus localhost) via CORS.
# The Go API already listens on 0.0.0.0:8080, so it's reachable on the Tailscale
# IP regardless. Unset → unchanged localhost behavior. (OAuth login from another
# device also needs the http://PUBLIC_HOST:8080/auth/callback redirect URI added
# in the Discord developer portal.)
PUBLIC_HOST ?=
comma := ,
VITE_HOST_FLAG := $(if $(PUBLIC_HOST),--host 0.0.0.0,)
# Env prefixes injected only when PUBLIC_HOST is set (empty otherwise, so the
# .env / localhost defaults are left untouched).
API_PUBLIC_ENV := $(if $(PUBLIC_HOST),API_BASE_URL=http://$(PUBLIC_HOST):8080 WEB_BASE_URL=http://$(PUBLIC_HOST):5173 CORS_ALLOW_ORIGINS=http://$(PUBLIC_HOST):5173$(comma)http://localhost:5173,)
WEB_PUBLIC_ENV := $(if $(PUBLIC_HOST),PUBLIC_API_URL=http://$(PUBLIC_HOST):8080 PUBLIC_WS_URL=ws://$(PUBLIC_HOST):8080/realtime,)

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z0-9_.-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

## ── One command ──────────────────────────────────────────────
.PHONY: up
up: ## Start the WHOLE dev stack: infra + migrations + seed + app. GATEWAY=1 adds gateway; SEED=0 skips fixtures
	$(MAKE) --no-print-directory infra
	@if [ "$(SEED)" != "0" ]; then \
		$(MAKE) --no-print-directory seed; \
	else \
		echo ">> SEED=0: applying migrations only (no fixtures)"; \
		$(MAKE) --no-print-directory migrate; \
	fi
	$(MAKE) --no-print-directory app
	@echo ""
	@echo "✓ Dia dev stack up. Dashboard http://localhost:5173 · API http://localhost:8080"
	@echo "  Stop: make down   ·   Wipe (incl. volumes): make reset   ·   Logs: make logs"

## ── Infra (shared, kept running) ─────────────────────────────
.PHONY: infra infra-up infra-down infra-logs
infra: ## Start shared infra (Postgres + Redis + NATS) and wait until healthy
	$(COMPOSE) up -d --wait $(INFRA_SVCS)
	@echo ""
	@echo "Infra healthy (project dia-dev). Switch worktrees freely; run 'make app'"
	@echo "(Docker) or the native targets, then 'make seed' for fixtures."

infra-up: infra ## Alias for `make infra`

infra-down: ## Stop infra (volumes preserved; 'make infra' brings it back)
	$(COMPOSE) stop $(INFRA_SVCS)

infra-logs: ## Follow infra logs
	$(COMPOSE) logs -f --tail=200 $(INFRA_SVCS)

## ── App (Docker) ─────────────────────────────────────────────
.PHONY: app app-down app-logs restart logs status down reset
app: ## Start app in Docker (worker api web). GATEWAY=1 adds gateway; SVCS="api web" picks a subset
	$(COMPOSE) up -d --build $(SVCS)
	@echo ""
	@echo "App up against infra. API :8080  ·  web :5173 (web has live HMR)."
	@echo "Apply Go/gateway code changes with: make restart SVC=<svc>"
	@echo "Logs: make app-logs   ·   Stop: make app-down   ·   Seed: make seed"

app-down: ## Stop app services (infra stays up)
	$(COMPOSE) stop $(APP_SVCS) gateway

app-logs: ## Follow app logs (one service: make app-logs SVC=api)
	$(COMPOSE) logs -f --tail=200 $(if $(SVC),$(SVC),$(SVCS))

restart: ## Recompile + restart one app service to pick up code changes: make restart SVC=api
	@if [ -z "$(SVC)" ]; then echo "Usage: make restart SVC=<service>"; exit 1; fi
	$(COMPOSE) up -d --build --force-recreate $(SVC)

logs: ## Follow all dev logs (one service: make logs SVC=api)
	$(COMPOSE) $(PROFILES) logs -f --tail=200 $(SVC)

status: ## Show dev container status
	$(COMPOSE) $(PROFILES) ps

down: ## Stop + remove dev containers (volumes preserved)
	$(COMPOSE) $(PROFILES) down

reset: ## Stop + remove dev containers AND volumes (start over)
	$(COMPOSE) $(PROFILES) down -v

## ── Seed / database ──────────────────────────────────────────
.PHONY: seed migrate db-wipe db-reset
seed: ## Load idempotent fixtures (runs migrations first). SEED_GUILD_ID=<id> targets your guild
	DATABASE_URL="$(DATABASE_URL)" SEED_GUILD_ID="$(SEED_GUILD_ID)" $(GO) run ./cmd/seed

migrate: ## Apply DB migrations and exit (no API server)
	DATABASE_URL="$(DATABASE_URL)" $(GO) run ./cmd/api --migrate-only

db-wipe: ## Drop every table in-place (re-apply with 'make migrate' or a service boot)
	$(COMPOSE) exec -T postgres psql -U dia -d dia -v ON_ERROR_STOP=1 \
		-c 'DROP SCHEMA public CASCADE; CREATE SCHEMA public; GRANT ALL ON SCHEMA public TO dia; GRANT ALL ON SCHEMA public TO public;'
	@echo ""
	@echo "Schema wiped. Run 'make migrate' (then 'make seed') to repopulate."

db-reset: ## Recreate the Postgres volume from scratch (nukes the DB only)
	$(COMPOSE) rm -sf postgres
	-docker volume rm dia-dev_pgdata
	$(COMPOSE) up -d postgres
	@echo ""
	@echo "Fresh Postgres up. Run 'make migrate && make seed' to repopulate."

## ── Full stack (prod-style smoke test) ───────────────────────
.PHONY: stack stack-down stack-logs
stack: ## Build + run the full self-hostable stack (deploy/docker-compose.yml)
	$(STACK_COMPOSE) up -d --build
	@echo "Full stack up. Dashboard :3000  ·  API :8080"

stack-down: ## Stop + remove the full stack (volumes preserved)
	$(STACK_COMPOSE) down

stack-logs: ## Follow full-stack logs (one service: make stack-logs SVC=api)
	$(STACK_COMPOSE) logs -f --tail=200 $(SVC)

## ── Go services (native) ─────────────────────────────────────
.PHONY: tidy build worker api run test vet fmt fonts
tidy: ## go mod tidy
	$(GO) mod tidy

build: ## Build the Go binaries into ./bin
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/worker ./cmd/worker
	$(GO) build -o $(BIN_DIR)/api    ./cmd/api
	$(GO) build -o $(BIN_DIR)/seed   ./cmd/seed

worker: ## Run the worker (bot brain) natively
	$(GO) run ./cmd/worker

api: ## Run the API (dashboard backend) natively. PUBLIC_HOST=<ip> exposes it off-box
	$(API_PUBLIC_ENV) $(GO) run ./cmd/api

run: ## Run worker + api together natively (Ctrl-C stops both). PUBLIC_HOST=<ip> exposes the API off-box
	@echo "worker + api (native, host Go). Ctrl-C stops both. Needs infra up ('make infra') and a .env."
	@trap 'kill 0' INT TERM; \
	$(MAKE) --no-print-directory worker & \
	$(MAKE) --no-print-directory api & \
	wait

test: ## Run Go tests
	$(GO) test ./internal/... ./cmd/...

vet: ## go vet
	$(GO) vet ./internal/... ./cmd/...

fmt: ## gofmt the Go code
	gofmt -w ./cmd ./internal

fonts: ## Download the curated card fonts into assets/fonts (idempotent)
	bash scripts/fetch-fonts.sh

## ── Gateway (Elixir, native) ─────────────────────────────────
.PHONY: gateway gateway-deps
gateway: ## Run the Elixir gateway natively (loads repo-root .env so DISCORD_TOKEN etc. reach mix)
	set -a; [ -f .env ] && . ./.env; set +a; cd gateway && mix run --no-halt

gateway-deps: ## Fetch Elixir deps
	cd gateway && mix deps.get

## ── Web (SvelteKit, native) ──────────────────────────────────
.PHONY: web web-install
web: ## Run the SvelteKit dev server natively (:5173). PUBLIC_HOST=<ip> exposes it off-box
	cd web && $(WEB_PUBLIC_ENV) pnpm dev $(VITE_HOST_FLAG)

web-install: ## Install web deps
	cd web && pnpm install
