import Config

# ── COMPILE-TIME configuration ──────────────────────────────────────────────
#
# Run Nostrum "thin": no caches at all. We are a stateless pump that forwards
# every relevant gateway event to NATS; the Go worker owns all state. Using the
# NoOp caches keeps memory flat and avoids any ETS/Mnesia churn under load.
#
# NOTE: the cache module is resolved at COMPILE TIME by Nostrum, so this MUST
# live in config.exs (not runtime.exs).
config :nostrum,
  caches: %{
    guilds: Nostrum.Cache.GuildCache.NoOp,
    members: Nostrum.Cache.MemberCache.NoOp,
    users: Nostrum.Cache.UserCache.NoOp,
    presences: Nostrum.Cache.PresenceCache.NoOp,
    channel_guild_mapping: Nostrum.Cache.ChannelGuildMapping.NoOp
  }

# We never make REST calls (token is used only for the gateway IDENTIFY), so the
# ratelimiter / REST machinery does not need to do anything fancy. Keep defaults.

config :logger, :console,
  metadata: [:shard, :guild_id, :event],
  level: :info

# Runtime-only settings (token, intents, sharding, NATS) live in runtime.exs so
# the same release binary can be configured purely via environment variables.
import_config "#{config_env()}.exs"
