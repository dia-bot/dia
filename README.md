<div align="center">
  <img src="web/static/favicon.svg" width="92" height="92" alt="Dia" />

  # Dia

  **A modern, open-source Discord bot with a beautifully simple realtime dashboard.**

  [![CI](https://github.com/dia-bot/dia/actions/workflows/ci.yml/badge.svg)](https://github.com/dia-bot/dia/actions/workflows/ci.yml)
</div>

---

## What is Dia?

Dia is a Discord bot you configure from a clean web dashboard. Everything is
slash-command native, every feature is fully customizable, and the dashboard is
**realtime**: create a channel in Discord and it shows up in the dropdowns
instantly. It's designed to be easy to self-host and to scale across machines
when your communities grow.

### Features

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

Dia is split into tiers, each using the right tool for the job:

- **Discord Gateway**: holds sharded WebSocket connections and sends normalized
  gateway events to `gateway/`.
- **Gateway (Elixir + Nostrum)**: the BEAM is ideal for thousands of supervised
  WebSocket shards. It holds the gateway connections, shards across nodes by
  config, and forwards every relevant event to NATS. No business logic.
- **NATS JetStream**: carries `discord.events.<type>.<guild_id>` messages for
  workers and realtime API consumers.
- **Worker (Go)**: consumes events, runs the feature plugins, routes slash-command
  interactions, awards XP, runs automod and renders images.
- **API (Go + gin)**: Discord OAuth2 login with Redis-backed sessions, per-guild
  configuration, welcome/rank image previews, and a realtime WebSocket that
  streams guild changes to the dashboard.
- **Web (SvelteKit + Svelte 5 + Tailwind v4)**: the landing page and dashboard.

### Tech stack

| Tier | Stack |
| --- | --- |
| Gateway | Elixir, Nostrum, gnat (NATS) |
| Event bus | NATS JetStream |
| Worker / API | Go, gin, pgx, go-redis, fogleman/gg (imaging), vendored discordgo (REST) |
| Database | PostgreSQL (durable config) + Redis (cache, sessions, realtime) |
| Web | SvelteKit 2, Svelte 5 (runes), TypeScript, Tailwind CSS v4 |

## Quick start (development)

```bash
cp .env.example .env          # fill in DISCORD_TOKEN, DISCORD_CLIENT_ID/SECRET, SESSION_SECRET
make infra-up                 # Postgres + Redis + NATS via docker

# in separate terminals:
make api                      # Go dashboard API on :8080 (runs migrations)
make worker                   # Go bot worker
make gateway-deps && make gateway   # Elixir gateway (needs DISCORD_TOKEN)
make web-install && make web        # SvelteKit dev server on :5173
```

Open http://localhost:5173 and log in with Discord.

## Quick start (Docker, full stack)

```bash
cp .env.example .env          # set DISCORD_* and SESSION_SECRET (openssl rand -hex 32)
docker compose -f deploy/docker-compose.yml up -d --build
```

Dashboard on http://localhost:3000, API on http://localhost:8080.

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
gateway/         Elixir gateway (Nostrum to NATS)
cmd/worker       Go bot worker (consumes events, runs plugins)
cmd/api          Go dashboard API (gin)
internal/        Go libraries: eventbus, store, discord, imaging, plugin SDK,
                 interactions, bot runtime, api, realtime, guildstate, features/*
pkg/discordgo    vendored Discord library (REST + types)
migrations/      versioned SQL (goose, embedded)
web/             SvelteKit landing + dashboard
deploy/          full-stack docker-compose + k8s
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
