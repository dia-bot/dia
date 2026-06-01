# ── Build stage ──────────────────────────────────────────────────────────────
# Pin to Elixir 1.18 / OTP 27 on Debian to match the development toolchain and
# the runtime base below (glibc compatibility for the compiled NIFs in the
# crypto deps that Nostrum pulls in).
FROM hexpm/elixir:1.18.4-erlang-27.3.4-debian-bookworm-20250407-slim AS build

# Build deps for native NIFs (kcl/curve25519/etc.) pulled in transitively.
RUN apt-get update -y \
    && apt-get install -y --no-install-recommends build-essential git \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Hex + Rebar
RUN mix local.hex --force && mix local.rebar --force

ENV MIX_ENV=prod

# Fetch deps first for better layer caching.
COPY mix.exs mix.lock ./
RUN mix deps.get --only prod
RUN mix deps.compile

# Compile-time config affects compilation (Nostrum cache modules), so copy it
# before compiling the app.
COPY config config
COPY lib lib

RUN mix compile

# Build the self-contained release.
RUN mix release dia_gateway

# ── Runtime stage ────────────────────────────────────────────────────────────
FROM debian:bookworm-slim AS runtime

# OpenSSL + locale + CA certs for TLS to Discord / NATS.
RUN apt-get update -y \
    && apt-get install -y --no-install-recommends libstdc++6 openssl libncurses6 locales ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && sed -i '/en_US.UTF-8/s/^# //g' /etc/locale.gen \
    && locale-gen

ENV LANG=en_US.UTF-8 \
    LANGUAGE=en_US:en \
    LC_ALL=en_US.UTF-8

# Run as a non-root user.
RUN useradd --create-home --uid 10001 --shell /usr/sbin/nologin dia
WORKDIR /app

COPY --from=build --chown=dia:dia /app/_build/prod/rel/dia_gateway ./

USER dia

# DISCORD_TOKEN, NATS_URL, NODE_COUNT, NODE_INDEX, SHARD_TOTAL etc. are provided
# at runtime via the environment (see README / .env.example).
ENTRYPOINT ["bin/dia_gateway"]
CMD ["start"]
