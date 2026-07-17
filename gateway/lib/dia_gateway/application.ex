defmodule Dia.Gateway.Application do
  @moduledoc """
  Top-level supervisor for the Dia gateway tier.

  The gateway is a thin, stateless pump: it holds the sharded Discord gateway
  WebSocket connections (via Nostrum, started as a dependency application) and
  forwards every relevant gateway event to NATS JetStream as JSON. It contains
  no command logic, no business logic and makes no Discord REST calls — all of
  that lives in the Go worker that consumes from NATS.

  Supervision tree (`:one_for_one`):

    1. (optional) `Cluster.Supervisor` — only when `CLUSTER_ENABLED=true`.
    2. A pool of `Gnat.ConnectionSupervisor` processes (`:gnat_0`..`:gnat_{n-1}`),
       one per scheduler by default, each reconnecting on its own backoff.
    3. `Dia.Gateway.Publisher` — facade over the pool. Ensures the JetStream
       stream exists on boot and publishes events with bounded retry. NATS
       failures are logged + metered but never crash the shard pipeline.
    4. `{Nostrum.Bot, platform_bot_options}` — the shared Dia bot, started as a
       Nostrum 0.11 multi-bot child (own token/intents/shards). Its `:consumer`
       is `Dia.Gateway.Consumer`; Nostrum owns the shard lifecycle.
    5. `{DynamicSupervisor, Dia.Gateway.BotSupervisor}` — holds the customers'
       custom bots, each its own `Nostrum.Bot` child added/removed at runtime.
    6. `Dia.Gateway.Control` — subscribes to the NATS control plane and
       starts/stops/restyles custom bots on command from the Go services.

  The consumer is NOT a supervised child in 0.11: it is just the module named by
  each bot's `:consumer`, invoked per event with that bot's context.
  """

  use Application

  require Logger

  # DynamicSupervisor that owns the custom-bot Nostrum.Bot children.
  @bot_supervisor Dia.Gateway.BotSupervisor

  @doc "Name of the custom-bot DynamicSupervisor (shared with Dia.Gateway.Control)."
  def bot_supervisor, do: @bot_supervisor

  @impl true
  def start(_type, _args) do
    children =
      cluster_child() ++
        nats_pool_children() ++
        [
          Dia.Gateway.Publisher,
          {Nostrum.Bot, platform_bot_options()},
          {DynamicSupervisor, strategy: :one_for_one, name: @bot_supervisor},
          Dia.Gateway.Control
        ]

    Logger.info("starting dia_gateway with #{length(children)} children")

    opts = [strategy: :one_for_one, name: Dia.Gateway.Supervisor]
    Supervisor.start_link(children, opts)
  end

  # The platform (shared Dia) bot's Nostrum 0.11 options. The token is wrapped in
  # a zero-arity fn so it never appears in a stacktrace. No :name is set, so it
  # defaults to the bot id decoded from the token; the consumer stamps that id as
  # the envelope's app_id and the Go side treats the platform id as "shared".
  defp platform_bot_options do
    cfg = Application.fetch_env!(:dia_gateway, :platform_bot)
    token = Keyword.fetch!(cfg, :token)

    %{
      consumer: Dia.Gateway.Consumer,
      intents: Keyword.fetch!(cfg, :intents),
      wrapped_token: fn -> token end,
      shards: Keyword.fetch!(cfg, :shards),
      # We never proactively request guild members; the worker does what it
      # needs via REST. Keeps us a pure pump and avoids member chunk traffic.
      request_guild_members: false
    }
  end

  # libcluster is compiled in but only started when explicitly enabled. Sharding
  # does NOT depend on clustering — it is driven purely by NODE_COUNT/NODE_INDEX.
  defp cluster_child do
    if Application.get_env(:dia_gateway, :cluster_enabled, false) do
      topologies = Application.get_env(:libcluster, :topologies, [])
      [{Cluster.Supervisor, [topologies, [name: Dia.Gateway.ClusterSupervisor]]}]
    else
      []
    end
  end

  # One Gnat.ConnectionSupervisor per pool slot. Each registers a named
  # connection (:gnat_0, :gnat_1, ...) that the Publisher round-robins over.
  defp nats_pool_children do
    cfg = Application.fetch_env!(:dia_gateway, Dia.Gateway.Publisher)
    pool_size = Keyword.fetch!(cfg, :pool_size)
    {host, port} = Dia.Gateway.Publisher.parse_url(Keyword.fetch!(cfg, :nats_url))

    for i <- 0..(pool_size - 1) do
      name = Dia.Gateway.Publisher.conn_name(i)

      settings = %{
        name: name,
        backoff_period: 2000,
        connection_settings: [
          %{host: host, port: port}
        ]
      }

      # Gnat.ConnectionSupervisor.start_link/2 takes (settings, gen_server_opts)
      # as two separate args, so we must use an explicit MFA child spec (the
      # `{Module, arg}` tuple form would pass a single combined arg and break).
      %{
        id: :"gnat_conn_sup_#{i}",
        start:
          {Gnat.ConnectionSupervisor, :start_link, [settings, [name: :"#{name}_supervisor"]]},
        type: :worker
      }
    end
  end
end
