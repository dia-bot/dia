# Contributing

Thanks for helping improve Dia. Keep changes focused, include the checks you ran
in the pull request, and avoid committing generated binaries or local build
output.

## Local setup

1. Copy environment defaults and fill in secrets:

   ```bash
   cp .env.example .env
   ```

2. Start the whole dev stack (infra + migrations + seed + app) in one command:

   ```bash
   make up      # stop later with `make down`
   ```

3. Or drive the pieces. `make help` lists every target.

   ```bash
   make infra                          # Postgres + Redis + NATS (shared)
   make seed                           # idempotent fixtures
   make app                            # Docker: worker + api (:8080) + web (:5173)
   # …or run it natively (no containers for the app):
   make run                            # worker + api together (Ctrl-C stops both)
   make web                            # SvelteKit dev server on :5173 (separate terminal)
   make gateway-deps && make gateway   # Elixir gateway, if needed
   ```

## Checks

Run the relevant checks before opening a pull request:

```bash
go test ./internal/... ./cmd/...
go vet ./internal/... ./cmd/...
cd gateway && mix format --check-formatted && mix compile --warnings-as-errors
cd web && pnpm install --frozen-lockfile && pnpm check && pnpm build
```

## Pull requests

- Open pull requests against `main`.
- Describe the user-facing change and any operational impact.
- Link related issues when available.
- Keep generated files, local binaries, secrets, logs, caches, and dependency
  directories out of Git.
- Update docs when commands, environment variables, or deployment behavior
  changes.
