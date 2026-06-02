<div align="center">
  <img src="web/static/favicon.svg" width="92" height="92" alt="Dia" />

  # Dia

  **A modern, open-source Discord bot with a realtime dashboard for teams that want control without bot sprawl.**

  [![CI](https://github.com/dia-bot/dia/actions/workflows/ci.yml/badge.svg)](https://github.com/dia-bot/dia/actions/workflows/ci.yml)
  [![Go Reference](https://pkg.go.dev/badge/github.com/dia-bot/dia.svg)](https://pkg.go.dev/github.com/dia-bot/dia)
  [![Go Version](https://img.shields.io/github/go-mod/go-version/dia-bot/dia)](https://go.dev/)
  [![Elixir](https://img.shields.io/badge/Elixir-1.18-4b275f?logo=elixir&logoColor=white)](https://elixir-lang.org/)
  [![SvelteKit](https://img.shields.io/badge/SvelteKit-2-ff3e00?logo=svelte&logoColor=white)](https://kit.svelte.dev/)
  [![Docker](https://img.shields.io/badge/Docker-ready-2496ed?logo=docker&logoColor=white)](deploy/docker-compose.yml)
  [![License](https://img.shields.io/badge/license-MIT-blue.svg)](#license)
</div>

---

## What is Dia?

Dia is a Discord bot you configure from a clean web dashboard. It is
slash-command native, fully self-hostable, and built around realtime guild state:
create a channel in Discord and it appears in the dashboard without a refresh.

It combines the features most communities install as separate bots into one
operational stack that can run locally, in Docker, or across multiple gateway
nodes when your communities grow.

## Highlights

| Area | What Dia provides |
| --- | --- |
| Dashboard | OAuth login, per-server settings, image previews, and live guild updates |
| Engagement | Welcome images, rank cards, XP, levels, leaderboards, and role rewards |
| Roles | Button/select reaction roles and automatic roles on member join |
| Moderation | Ban, kick, timeout, warn, case logs, spam rules, invite filters, link filters, and banned words |
| Commands | Custom slash commands designed from the dashboard |
| Operations | Docker Compose, embedded migrations, Redis cache, Postgres storage, and NATS-backed event delivery |

## Features

- **Welcome**: greet members with custom messages and rendered welcome-card
  images. Pick a preset or design your own with a live preview.
- **Leveling**: XP, levels, role rewards, leaderboards and generated rank cards.
- **Reaction & auto roles**: self-assignable roles via buttons and select
  menus, plus automatic roles on join.
- **Moderation & automod**: ban / kick / timeout / warn with a case log, and
  rule-based automod for spam, invites, links and banned words.
- **Custom commands**: design your own slash commands from the dashboard, no
  code required.

## Architecture

Dia is split into tiers, each using the right tool for the job. The gateway only
normalizes Discord events, the worker owns bot behavior, and the API owns the
dashboard contract.

| Tier | Stack |
| --- | --- |
| Discord gateway | Sharded WebSocket connections through Elixir and Nostrum |
| Event bus | NATS JetStream subjects like `discord.events.<type>.<guild_id>` |
| Worker | Go plugins for interactions, XP, automod, roles, welcome images, and custom commands |
| API | Go and gin for OAuth, sessions, config CRUD, previews, and realtime WebSocket updates |
| Data | PostgreSQL for durable config, Redis for cache, sessions, and live guild snapshots |
| Web | SvelteKit 2, Svelte 5 runes, TypeScript, and Tailwind CSS v4 |

## Tech stack

| Layer | Tools |
| --- | --- |
| Gateway | Elixir, Nostrum, gnat |
| Backend | Go, gin, pgx, go-redis, goose |
| Bot SDK | Internal plugin framework plus vendored Discord REST client |
| Imaging | fogleman/gg and image helpers for welcome and rank cards |
| Frontend | SvelteKit, Svelte 5, TypeScript, Tailwind CSS |
| Infrastructure | Docker Compose, Postgres, Redis, NATS JetStream |

## Quick start (development)

The fastest path is one command: `make up` brings up infra, applies
migrations, seeds fixtures, and starts the app. `make down` stops it.

```bash
cp .env.example .env          # fill in DISCORD_TOKEN, DISCORD_CLIENT_ID/SECRET, SESSION_SECRET
make up                       # infra + migrations + seed + app (the whole dev stack)
# …later…
make down                     # stop everything (volumes kept; `make reset` wipes them)
```

`make up GATEWAY=1` also starts the Elixir gateway; `make up SEED=0` skips the
fixtures (migrations still run). Prefer to drive the pieces? `make infra` brings
up the shared stateful services (Postgres, Redis, NATS) and leaves them running,
`make seed` loads idempotent fixtures, and `make app` starts the app (or run it
natively). `make help` lists every target.

**Docker.** Source is bind-mounted and build caches are shared across git
worktrees. The web app has live HMR (Vite); the Go services run via `go run`
and the gateway via `mix run`, which don't auto-reload inside Docker (host→VM
file-change events are unreliable), so apply code changes with
`make restart SVC=api` — it recompiles from the mounted source against the warm
cache in a few seconds. Every container reads your repo-root `.env` (via
`env_file`), so credentials and any supported variable you set there flow
straight through; the in-container infra URLs (Postgres/Redis/NATS) are the only
values the compose file overrides.

```bash
make app                      # worker + api (:8080) + web (:5173)
make app GATEWAY=1            # also start the Elixir gateway (needs DISCORD_TOKEN)
make restart SVC=api          # recompile + restart api after editing Go code
make app-logs                 # follow logs   ·   make app-down
```

**Native (host Go/Node).** Fastest inner loop — no containers for the app.

```bash
make run                      # worker + api together on the host (Ctrl-C stops both)
make web                      # SvelteKit dev server on :5173 (separate terminal)
make gateway-deps && make gateway   # Elixir gateway, if you need it (needs DISCORD_TOKEN)
# …or run the Go services one at a time:
make worker                   # just the bot worker
make api                      # just the API on :8080 (runs migrations)
```

`make run` and `make web` use plain `go run` / `pnpm dev` on the host (no
Docker); they talk to the infra started by `make infra` and to each other on
`localhost`.

To reach the dashboard from another device — over Tailscale or the LAN — set
`PUBLIC_HOST` to this machine's Tailscale IP or MagicDNS name and pass it to the
native targets:

```bash
make run PUBLIC_HOST=$(tailscale ip -4 | head -1)
make web PUBLIC_HOST=$(tailscale ip -4 | head -1)
```

The web dev server then binds `0.0.0.0`, the dashboard calls the API at
`PUBLIC_HOST:8080`, and the API allows that origin via CORS (localhost keeps
working too). OAuth login from another device additionally needs
`http://PUBLIC_HOST:8080/auth/callback` registered as a redirect URI in the
Discord developer portal.

Open http://localhost:5173 and log in with Discord. To make the seeded
configuration show up for a server you own, point the seeder at it:
`make seed SEED_GUILD_ID=<your guild id>` (the dashboard only lists guilds the
bot is in).

## Quick start (Docker, full stack)

The full self-hostable stack (production-style images for every service) lives
in `deploy/docker-compose.yml`. `make stack` is a thin wrapper around it that
also feeds it your repo-root `.env`.

```bash
cp .env.example .env          # set DISCORD_* and SESSION_SECRET (openssl rand -hex 32)
make stack                    # == docker compose --env-file .env -p dia-stack -f deploy/docker-compose.yml up -d --build
```

Dashboard on http://localhost:3000, API on http://localhost:8080. Stop it with
`make stack-down`. This runs under its own compose project (`dia-stack`) so it
never collides with the `make infra` / `make app` dev stack.

> The `--env-file .env` flag matters: because the compose file is under
> `deploy/`, a bare `docker compose -f deploy/docker-compose.yml up` looks for
> `deploy/.env` and ignores your repo-root `.env`. Use `make stack` (which adds
> the flag) or pass `--env-file .env` yourself.

## Scaling across machines

Run the Elixir gateway on multiple nodes and split the shards by config. No code
changes:

```bash
SHARD_TOTAL=16 NODE_COUNT=4 NODE_INDEX=0  # node 0 owns shards 0-3
SHARD_TOTAL=16 NODE_COUNT=4 NODE_INDEX=1  # node 1 owns shards 4-7, etc.
```

The Go worker and API are stateless and scale horizontally behind NATS durable
consumers.

## Project structure

```text
gateway/                 Elixir gateway (Nostrum to NATS)
cmd/worker               Go bot worker (consumes events, runs plugins)
cmd/api                  Go dashboard API (gin)
cmd/seed                 Go dev seeder (idempotent fixture data)
internal/                Go libraries: eventbus, store, discord, imaging, plugin
                         SDK, interactions, bot runtime, api, realtime, features/*
pkg/discordgo            vendored Discord library (REST + types)
migrations/              versioned SQL (goose, embedded)
web/                     SvelteKit landing + dashboard
docker-compose.yml       local dev stack: infra + app services via compose
                         profiles (`make infra` / `make app`)
deploy/                  full self-hostable stack (`make stack`) + dev Dockerfiles
```

### Extending Dia

Features are plugins implementing a tiny SDK (`internal/plugin`). A plugin
declares its slash commands, component/modal handlers, event subscriptions and
background workers in `Init`, and stores its config as JSON keyed by a feature
key. See `internal/features/welcome` for the canonical example.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for local setup, checks, and pull request
guidelines.

## License

MIT
