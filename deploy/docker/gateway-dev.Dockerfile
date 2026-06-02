# Elixir dev image for the gateway.
#
# Unlike the production gateway Dockerfile (which cuts a release), this runs the
# app straight from source via `mix run --no-halt`, with deps and _build on
# named volumes. There is no Phoenix code-reloader here, so after changing
# Elixir source restart the container: `make restart gateway`.
#
# Pinned to the same Elixir/OTP/Debian as the production image for NIF/glibc
# parity (Nostrum pulls in compiled crypto NIFs).
FROM hexpm/elixir:1.18.4-erlang-27.3.4-debian-bookworm-20250407-slim

# Build deps for native NIFs (kcl/curve25519/etc.) pulled in transitively, plus
# git for hex deps fetched from source.
RUN apt-get update -y \
    && apt-get install -y --no-install-recommends build-essential git \
    && rm -rf /var/lib/apt/lists/*

RUN mix local.hex --force && mix local.rebar --force

WORKDIR /app

ENV MIX_ENV=dev
