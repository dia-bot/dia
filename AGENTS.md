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
| `cmd/seed` | Idempotent dev fixture loader (`make seed`) | Go |
| `internal/` | Go libraries for event, store, discord, imaging, plugin SDK, interactions, bot, api, realtime, guildstate, and features | Go |
| `pkg/discordgo` | Vendored Discord library in-module | Go |
| `migrations/` | Versioned SQL with goose and embedded migrations | SQL |
| `web/` | SvelteKit landing page and dashboard | TS/Svelte |
| `docker-compose.yml` | Local dev stack: infra + app services via compose profiles; `make infra` / `make app` | Docker |
| `deploy/` | Full self-hostable stack + dev Dockerfiles | Docker |

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
- **Web theme is serious & technical, not gradient-heavy.** A neutral
  charcoal/paper palette (`--color-ink` near-black on `--color-bg`/`--color-surface`),
  hairline rules (`--color-line`), and a single rose accent (`--color-accent` = #ff6363, the logo's top colour; `--color-accent-ink` is the deeper rose for text/links).
  Native system font for UI (`--font-sans` = the OS system stack: Apple
  system / San Francisco, Segoe UI, Roboto, etc.), sitewide across dashboard &
  marketing; monospace for technical labels & code (`--font-mono` = JetBrains
  Mono); `.eyebrow` is the mono label style. The
  pink→purple gradient is for the **logo mark and welcome/rank cards only** — never a
  page/section/dashboard background (use near-black `--color-ink-2` sections for
  emphasis instead). Marketing site lives in `web/src/routes` (home + `/features/*`,
  `/pricing`, `/compare`, `/about`, `/contact`, `/terms`, `/privacy`) with shared
  pieces in `web/src/lib/components/marketing`. Svelte 5 runes
  (`$state`/`$derived`/`$effect`/`$props`); reuse the components in
  `web/src/lib/components`.
- **Never reference other bots/competitors by name** anywhere in code, comments,
  or UI copy.
- **Avoid em-dashes (—) in prose** — UI copy, docs, comments, commit messages.
  Prefer commas, periods, or parentheses; reach for an em-dash only when the
  sentence genuinely needs one.

## Custom bots ("bring your own token")

A customer can run Dia under their own Discord application: they supply a bot
token, we run their bot on our infra so their server's bot wears their name,
avatar and (unlike the shared bot) its own presence, while every feature still
works. Architecture:

- **Gateway (Elixir, Nostrum 0.11 multi-bot).** The platform bot and each custom
  bot are `Nostrum.Bot` children; custom ones live under the
  `Dia.Gateway.BotSupervisor` DynamicSupervisor. `Dia.Gateway.Control` subscribes
  to the NATS control plane and starts/stops/restyles bots. Every forwarded event
  is stamped with `app_id` (the producing bot). Nostrum multi-bot only exists on
  0.11, so `mix.exs` pins a 0.11-dev git ref (bump when 0.11 hits Hex).
- **Control plane (contract in `internal/event/control.go`, mirrored in
  `gateway/lib/dia_gateway/control.ex`).** Core NATS (latest-wins + reconcile),
  NOT the durable JetStream event stream. Go publishes `ensure`/`remove`/
  `presence` on `dia.control.bots`; the gateway reports `ready`/`bot_state` on
  `dia.control.gateway`. `internal/custombot` owns the Go side (Manager publishes,
  the worker Service reconciles on gateway hello + a 60s tick and registers the
  command set under a custom app when it first reports ready).
- **Per-guild REST token.** `internal/botreg.Registry` resolves the client that
  should act for a guild/app; the worker injects it into the request context from
  the event's `app_id` (`discord.WithClient`). **When a feature sends a message,
  grants a role, or takes any non-interaction REST action in response to a guild
  event, use `d.ClientFor(ctx, guildID)`, never `d.Discord`** — a custom-bot
  guild may not have the shared bot in it at all, so the shared token would 403.
  Interaction *responses* are exempt: they're authenticated by the interaction
  token + app id in the URL, so `c.Client` already works. `internal/features/welcome`
  is the reference conversion; other features should follow the same pattern.
- **Secrets.** Tokens are encrypted at rest with `internal/secret` (AES-256-GCM,
  key from `CUSTOM_BOT_ENC_KEY`). Never log or return a token; the dashboard view
  is `customBotView` (no secrets). Custom bots are admin-only.

## Custom commands: the templating contract

Every user-facing **string value in a custom-command definition is a Go
`text/template`**, rendered at runtime against the run's scope
(`internal/features/customcommands/scope.go`). That covers message content,
embed titles/descriptions/fields, reasons, nicknames, URLs, KV keys — both
plain "templated string" spec fields and `Expr` values (`{lang:"tmpl",
src:"…"}`). If a step needs a value pulled into a string, it goes through a
template; never invent a second interpolation syntax.

Values in scope inside any template:

- `{{ .Input.<name> }}` — the slash **property** values, keyed by option name.
- `{{ .Vars.<name> }}` — declared variables and anything written by `set_var`
  / `into` fields.
- `{{ .User.* }}` (`ID`, `Username`, `GlobalName`, `Mention`, `Bot`),
  `{{ .Member.* }}` (`Nick`, `Roles`, `JoinedAt`), `{{ .Guild.* }}` (`ID`,
  `Name`, `MemberCount`), `{{ .Channel.ID }}`, `{{ .Now }}`, `{{ .Last }}`.
- `{{ .Error.* }}` — only inside `on_error` subtrees.

**Go template syntax is the only syntax, everywhere.** Every placeholder,
picker token, step default, hint, fixture and doc example must be a Go
template (`{{ .User.Mention }}`, `{{ .Input.amount }}`), with no exceptions.
The brace shorthands (`{user.mention}`, `{server}`, `{input.<name>}`, …) are
legacy input sugar: the runtime still expands them so old saved definitions
keep working, but nothing may display, insert, generate or document them.
Read-side compatibility (recognising a legacy value in a stored spec) is the
single allowed appearance.

Keep these mirrors in lockstep when touching either side:

- Definition shapes: `internal/features/customcommands/{config,kinds}.go` ↔
  `web/src/lib/commands/types.ts` (the editor must never produce JSONB the
  runtime won't decode — message/embed/component editors mirror `SpecReply` /
  `EmbedSpec` / `ComponentRow` exactly).
- Template functions & scope vars: `internal/templating/funcs.go` ↔
  `web/src/lib/commands/expr-meta.ts` (drives the dashboard's variable /
  function pickers).

**Templates are pure.** Template functions only read values and format
strings (lookups like `getRole`/`getChannel` are read-only). Never add a
side-effecting template function — anything that *does* something (send,
grant, react, …) must be a custom-command **step** so it's visible on the
canvas, validated, and budgeted.

## Where things live

- Add a DB change: new file in `migrations/` (`NNNNN_name.sql`, goose
  `-- +goose Up/Down`); it's embedded and applied at startup.
- Add an API endpoint: register the route in `internal/api/server.go`, handler in
  the matching `internal/api/*.go`.
- Add a dashboard page: `web/src/routes/servers/[id]/<feature>/+page.svelte`,
  following `welcome/+page.svelte`; link it in `[id]/+layout.svelte`.
- Add dev fixtures: extend `cmd/seed` (reuse the feature's `Default()` so the
  seeded JSONB can't drift). Keep every write idempotent (upsert or existence
  guard) — `make seed` is meant to be re-runnable.
