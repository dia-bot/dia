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
  Grotesk for UI (`--font-sans` = Hanken Grotesk), monospace for technical labels &
  code (`--font-mono` = JetBrains Mono); `.eyebrow` is the mono label style. The
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
- Brace shorthands expand too: `{user.mention}`, `{user.id}`, `{server}`,
  `{channel}`, `{input.<name>}`, `{vars.<name>}`.

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
