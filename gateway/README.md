# Dia Gateway

The Elixir gateway tier for **Dia**, a modern open-source Discord bot.

This service is a **thin, stateless pump**. It holds the sharded Discord gateway
WebSocket connections (via [Nostrum](https://github.com/Kraigie/nostrum)) and
forwards every relevant gateway event to **NATS JetStream** as JSON. It contains:

- **no command logic**
- **no business logic**
- **no Discord REST calls** (the bot token is used only for the gateway IDENTIFY)

All logic lives in a separate **Go worker** that consumes from NATS. The JSON
contract is defined by the Go `internal/event` package and is the single source
of truth; this gateway re-serializes Nostrum's structs to match it byte-for-byte
(all snowflake IDs are decimal **strings**).

## Architecture

Discord gateway events flow through Nostrum shards into `Dia.Gateway.Consumer`,
then through `Dia.Gateway.Mapper` into normalized envelope JSON. The publisher
pool writes those envelopes to NATS JetStream on
`discord.events.<TYPE>.<guild_id>` subjects for Go workers to consume.

### Supervision tree (`:one_for_one`)

1. `Cluster.Supervisor`: **only** when `CLUSTER_ENABLED=true` (libcluster is
   compiled in but off by default; it is *not* required for sharding).
2. A pool of `Gnat.ConnectionSupervisor` processes (`:gnat_0`..`:gnat_{n-1}`),
   one per scheduler by default, each with a 2s reconnect backoff.
3. `Dia.Gateway.Publisher`: facade over the pool. Ensures the JetStream stream
   exists on boot and publishes with bounded retry. **NATS failures never crash
   the shard pipeline** (retry, then drop + telemetry).
4. **Nostrum** runs as a dependency application and owns the shard lifecycle
   (IDENTIFY / RESUME / backoff). We do not supervise shards ourselves.
5. `Dia.Gateway.Consumer`: a single `Nostrum.Consumer` whose catch-all
   `handle_event/1` maps and publishes each forwarded event.

## Forwarded events

`GUILD_CREATE`, `GUILD_UPDATE`, `GUILD_DELETE`, `CHANNEL_CREATE`,
`CHANNEL_UPDATE`, `CHANNEL_DELETE`, `GUILD_ROLE_CREATE`, `GUILD_ROLE_UPDATE`,
`GUILD_ROLE_DELETE`, `GUILD_MEMBER_ADD`, `GUILD_MEMBER_REMOVE`,
`GUILD_MEMBER_UPDATE`, `MESSAGE_CREATE`, `INTERACTION_CREATE`.

All other events are ignored.

### Envelope

Every message body is:

```json
{
  "type": "MESSAGE_CREATE",
  "guild_id": "123...",        // "" when absent
  "shard_id": 0,
  "ts": 1717200000000,          // unix milliseconds when forwarded
  "data": { /* normalized payload, per internal/event */ }
}
```

The NATS **subject** is `discord.events.<TYPE>.<guild_id>`, where an empty guild
id maps to the `0` token. Each publish carries a `Nats-Msg-Id` header (message
id / interaction id / a derived id) for JetStream dedupe within the duplicate
window.

## Environment variables

| Var | Default | Description |
| --- | --- | --- |
| `DISCORD_TOKEN` | **(required)** | Bot token (used only for IDENTIFY). |
| `GATEWAY_INTENTS` | `guilds,guild_members,guild_messages,message_content` | Comma/space-separated intent atoms. |
| `NODE_COUNT` | `1` | Number of gateway nodes sharing the global shard set. |
| `NODE_INDEX` | `0` | This node's 0-based index in `0..NODE_COUNT-1`. |
| `SHARD_TOTAL` | `1` | Global Discord shard count. |
| `NATS_URL` | `nats://localhost:4222` | NATS server URL (`host:port`). |
| `NATS_POOL_SIZE` | `System.schedulers_online()` | Number of pooled NATS connections. |
| `NATS_STREAM` | `DIA_EVENTS` | JetStream stream name. |
| `NATS_STREAM_MAX_AGE_SECONDS` | `86400` (24h) | Stream `max_age`. |
| `NATS_STREAM_DUP_WINDOW_SECONDS` | `120` (2m) | Stream duplicate window. |
| `NATS_PUBLISH_RETRIES` | `3` | Bounded publish retry attempts. |
| `CLUSTER_ENABLED` | `false` | Start libcluster (Gossip strategy by default). |

### Config-driven multi-node sharding

Each node owns a **contiguous** slice of the global shard range, computed
deterministically from `(NODE_COUNT, NODE_INDEX, SHARD_TOTAL)` with an even split
and the remainder distributed to the first nodes:

```
base       = div(SHARD_TOTAL, NODE_COUNT)
extra      = rem(SHARD_TOTAL, NODE_COUNT)
count_here = base + (NODE_INDEX < extra ? 1 : 0)
first_0b   = NODE_INDEX * base + min(NODE_INDEX, extra)
```

This is passed to Nostrum as `num_shards`:

- `NODE_COUNT == 1 and SHARD_TOTAL == 1`: `:auto` (Discord picks the count).
- `count_here == 0` (more nodes than shards): `:manual` (connects nothing).
- otherwise: `{first_0b + 1, first_0b + count_here, SHARD_TOTAL}` (Nostrum's
  1-based inclusive tuple; it connects 0-based Discord ids
  `(lowest-1)..(highest-1)`).

Example with `SHARD_TOTAL=7`, `NODE_COUNT=3`:

| Node | `num_shards` | Discord shard ids |
| --- | --- | --- |
| 0 | `{1, 3, 7}` | 0,1,2 |
| 1 | `{4, 5, 7}` | 3,4 |
| 2 | `{6, 7, 7}` | 5,6 |

## Running

### Development

```sh
export DISCORD_TOKEN=your-bot-token
# (a local NATS server with JetStream enabled, e.g. `nats-server -js`)
mix deps.get
iex -S mix
```

The gateway connects shards and forwards events to NATS. With no NATS reachable
it still boots; the publisher retries the stream creation in the background and
drops events until a connection is available (shards stay up regardless).

### Production (release)

```sh
MIX_ENV=prod mix release dia_gateway
DISCORD_TOKEN=... NATS_URL=nats://nats:4222 _build/prod/rel/dia_gateway/bin/dia_gateway start
```

### Docker

```sh
docker build -t dia-gateway .
docker run --rm \
  -e DISCORD_TOKEN=... \
  -e NATS_URL=nats://nats:4222 \
  -e SHARD_TOTAL=2 \
  dia-gateway
```

The image is a multi-stage build (hexpm/elixir build stage to `debian:bookworm-slim`
runtime), runs as a non-root user, and uses `bin/dia_gateway` as its entrypoint.

## Notes on the "thin" / NoOp-cache trade-off

Because all Nostrum caches are NoOp, a few delete-style events lose data that a
cache would otherwise have retained, and are therefore **not forwardable**:

- `GUILD_MEMBER_REMOVE` and `GUILD_ROLE_DELETE` and `CHANNEL_DELETE` arrive as
  `:noop` (the NoOp cache's delete return), with no recoverable id.
- `GUILD_DELETE` arrives without the guild id (the NoOp guild cache returns
  `nil` for the deleted guild); the unavailability bit is still forwarded.
- Member structs only retain `user_id` (not the full nested user), so
  `member.user` on `GUILD_MEMBER_ADD` / `GUILD_MEMBER_UPDATE` carries only `id`.

If a worker needs full fidelity on these, enable the corresponding real Nostrum
cache in `config/config.exs`.
