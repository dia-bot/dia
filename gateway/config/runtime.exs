import Config

# ── RUNTIME configuration ───────────────────────────────────────────────────
#
# Everything here is read from environment variables so a single release binary
# can be deployed unchanged across dev / staging / prod and across N gateway
# nodes. No secrets or topology are baked into the build.

# Small helpers -------------------------------------------------------------

get_int = fn name, default ->
  case System.get_env(name) do
    nil -> default
    "" -> default
    v -> String.to_integer(String.trim(v))
  end
end

get_bool = fn name, default ->
  case System.get_env(name) do
    nil -> default
    v -> String.downcase(String.trim(v)) in ["1", "true", "yes", "on"]
  end
end

# ── Discord token + intents ────────────────────────────────────────────────

token = System.fetch_env!("DISCORD_TOKEN")

gateway_intents =
  System.get_env(
    "GATEWAY_INTENTS",
    # guild_message_reactions / guild_voice_states / guild_moderation power the
    # reaction, voice and ban automation triggers (all non-privileged).
    "guilds,guild_members,guild_messages,message_content,guild_message_reactions,guild_voice_states,guild_moderation"
  )
  |> String.split([",", " "], trim: true)
  |> Enum.map(&String.to_atom/1)

# ── Config-driven multi-node sharding ──────────────────────────────────────
#
# Each gateway node owns a contiguous slice of the global shard range. The slice
# is computed deterministically from (NODE_COUNT, NODE_INDEX, SHARD_TOTAL) so
# that, given the same SHARD_TOTAL, the union of all nodes' slices is exactly the
# full 0..SHARD_TOTAL-1 range with no gaps or overlaps.
#
# Nostrum's `num_shards: {lowest, highest, total}` is 1-based inclusive; Discord
# shard ids are 0-based, and Nostrum connects ids (lowest-1)..(highest-1).

node_count = max(get_int.("NODE_COUNT", 1), 1)
node_index = get_int.("NODE_INDEX", 0)
shard_total = max(get_int.("SHARD_TOTAL", 1), 1)

base = div(shard_total, node_count)
extra = rem(shard_total, node_count)
count_here = base + if(node_index < extra, do: 1, else: 0)
first_0b = node_index * base + min(node_index, extra)

num_shards =
  cond do
    # Single node, single shard: let Discord pick the recommended count.
    node_count == 1 and shard_total == 1 -> :auto
    # This node owns no shards (more nodes than shards). Connect nothing.
    count_here == 0 -> :manual
    # Own the contiguous slice [first_0b, first_0b + count_here) as a 1-based
    # inclusive tuple, with the global total appended.
    true -> {first_0b + 1, first_0b + count_here, shard_total}
  end

config :nostrum,
  token: token,
  gateway_intents: gateway_intents,
  num_shards: num_shards,
  # We never proactively request guild members; the worker does what it needs
  # via REST. Keeps us a pure pump and avoids large member chunk traffic.
  request_guild_members: false

# ── NATS / JetStream ────────────────────────────────────────────────────────

schedulers = System.schedulers_online()

config :dia_gateway, Dia.Gateway.Publisher,
  nats_url: System.get_env("NATS_URL", "nats://localhost:4222"),
  pool_size: max(get_int.("NATS_POOL_SIZE", schedulers), 1),
  stream_name: System.get_env("NATS_STREAM", "DIA_EVENTS"),
  stream_subjects: ["discord.events.>"],
  # 24h retention, 2m duplicate window — must match the Go consumer's stream.
  max_age_seconds: get_int.("NATS_STREAM_MAX_AGE_SECONDS", 24 * 60 * 60),
  duplicate_window_seconds: get_int.("NATS_STREAM_DUP_WINDOW_SECONDS", 120),
  publish_retries: get_int.("NATS_PUBLISH_RETRIES", 3)

# ── libcluster (compiled in, OFF by default) ────────────────────────────────
#
# Enable with CLUSTER_ENABLED=true. The default strategy is Gossip; override via
# the topology env vars if you run on Kubernetes etc. Clustering is NOT required
# for sharding (that is driven purely by NODE_COUNT/NODE_INDEX above) — it only
# matters if you later add cross-node coordination.
if get_bool.("CLUSTER_ENABLED", false) do
  config :dia_gateway, :cluster_enabled, true

  config :libcluster,
    topologies: [
      dia_gateway: [
        strategy: Cluster.Strategy.Gossip
      ]
    ]
else
  config :dia_gateway, :cluster_enabled, false
end
