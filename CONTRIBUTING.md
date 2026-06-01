# Contributing

Thanks for helping improve Dia. Keep changes focused, include the checks you ran
in the pull request, and avoid committing generated binaries or local build
output.

## Local setup

1. Copy environment defaults and fill in secrets:

   ```bash
   cp .env.example .env
   ```

2. Start local infrastructure:

   ```bash
   make infra-up
   ```

3. Run the services you need:

   ```bash
   make api
   make worker
   make gateway-deps && make gateway
   make web-install && make web
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
