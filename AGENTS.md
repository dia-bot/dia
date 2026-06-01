# Dia Agent Guide

Guidance for AI coding agents working in this repo. For the architecture and how
to run things, see [README.md](README.md). This file is about **how to work
here**, not how to deploy.

## Repo layout (quick reference)

| Path | Purpose | Stack |
| --- | --- | --- |
| `gateway/` | Elixir gateway from Nostrum to NATS | Elixir |
| `cmd/worker` | Go bot worker for events and plugins | Go |
| `cmd/api` | Go dashboard API with gin | Go |
| `internal/` | Go libraries for event, store, discord, imaging, plugin SDK, interactions, bot, api, realtime, guildstate, and features | Go |
| `pkg/discordgo` | Vendored Discord library in-module | Go |
| `migrations/` | Versioned SQL with goose and embedded migrations | SQL |
| `web/` | SvelteKit landing page and dashboard | TS/Svelte |
| `deploy/` | docker-compose and Dockerfiles | Docker |

## Before you finish: format & check

Run only for the area you touched:

- **Go**: format and vet the changed packages:
  ```bash
  gofmt -w <changed .go files>          # or: gofmt -l ./internal ./cmd  (lists unformatted)
  go vet ./internal/... ./cmd/...       # built-in linter; fast
  ```
- **Elixir** (inside `gateway/`):
  ```bash
  mix format
  ```
- **Web** (inside `web/`): type-check is the lint here:
  ```bash
  pnpm exec svelte-check --tsconfig ./tsconfig.json
  ```

If `golangci-lint` is installed, `golangci-lint run ./internal/... ./cmd/...` is
welcome; it is not required.

## Do NOT run these

- **Do not build the whole thing.** No `go build ./...`, no building the binaries
  just to check. `go vet` already type-checks. Build only the single package you
  changed if you must (`go build ./internal/<pkg>/`).
- No `pnpm build` (full production web build). `svelte-check` is enough.
- No `mix compile` / `mix release` / `mix deps.get` in `gateway/` unless you
  changed Elixir deps (these pull from hex and are slow).
- No `go mod tidy` unless you actually changed Go dependencies.
- Do not start services, run the bot, or `docker compose up`. Don't run the
  gateway/worker/api/web dev servers to "verify". Leave running things to the user.

## Conventions

- **Commits:** continuous, **single-line** conventional messages, **no trailers**
  (no `Co-Authored-By`, no body). e.g. `feat(api): add rank preview endpoint`.
- **Slash-command native only.** No prefix/message commands.
- **Features are plugins.** A feature implements the tiny SDK in `internal/plugin`
  and declares its commands / component+modal handlers / event subscriptions /
  background workers in `Init`. Config is stored as JSONB keyed by the feature
  key (`guild_feature_configs`). Copy `internal/features/welcome` as the template.
- **Event contract:** `internal/event` is the single source of truth for the
  gateway↔Go boundary (all snowflake IDs are decimal strings). If you change a
  payload shape, update the Elixir mapper in `gateway/lib/dia_gateway/mapper.ex`
  to match.
- **Module path** is `github.com/dia-bot/dia`. Import the Discord library as
  `github.com/dia-bot/dia/pkg/discordgo` (it's vendored in-module; there is **no**
  `replace` directive. Don't add one).
- **Web theme is clean, not gradient-heavy.** White/blush surfaces, hairline
  borders, a single purple accent (`--color-accent`). The pink to purple gradient is
  for the **logo and welcome/rank cards only**. Never a page/dashboard
  background. Svelte 5 runes (`$state`/`$derived`/`$effect`/`$props`); reuse the
  components in `web/src/lib/components`.
- **Never reference other bots/competitors by name** anywhere in code, comments,
  or UI copy.

## Where things live

- Add a DB change: new file in `migrations/` (`NNNNN_name.sql`, goose
  `-- +goose Up/Down`); it's embedded and applied at startup.
- Add an API endpoint: register the route in `internal/api/server.go`, handler in
  the matching `internal/api/*.go`.
- Add a dashboard page: `web/src/routes/servers/[id]/<feature>/+page.svelte`,
  following `welcome/+page.svelte`; link it in `[id]/+layout.svelte`.
