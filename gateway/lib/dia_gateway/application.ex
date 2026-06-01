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
    4. Nostrum itself runs as a dependency application and owns the shard
       lifecycle (IDENTIFY / RESUME / backoff). We do NOT supervise shards.
    5. `Dia.Gateway.Consumer` — a `Nostrum.Consumer` whose single catch-all
       `handle_event/1` maps each event and hands it to the Publisher.
  """

  use Application

  require Logger

  @impl true
  def start(_type, _args) do
    children =
      cluster_child() ++
        nats_pool_children() ++
        [
          Dia.Gateway.Publisher,
          Dia.Gateway.Consumer
        ]

    Logger.info("starting dia_gateway with #{length(children)} children")

    opts = [strategy: :one_for_one, name: Dia.Gateway.Supervisor]
    Supervisor.start_link(children, opts)
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

      Supervisor.child_spec(
        {Gnat.ConnectionSupervisor, [settings, [name: :"#{name}_supervisor"]]},
        id: :"gnat_conn_sup_#{i}"
      )
    end
  end
end
