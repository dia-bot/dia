defmodule Dia.Gateway.Publisher do
  @moduledoc """
  Facade over the NATS connection pool.

  Responsibilities:

    * On boot, idempotently ensure the JetStream stream exists (matching the
      shape the Go consumer expects: file storage, limits retention, 24h max
      age, 2m duplicate window, subjects `discord.events.>`).
    * Publish each forwarded event to its subject with a bounded retry, setting
      the `Nats-Msg-Id` header for server-side dedupe within the duplicate
      window.

  Crucially, a publish failure is **logged + metered and then dropped**. It must
  never crash the shard pipeline — losing an event is strictly preferable to
  tearing down a gateway connection (which would trigger a reconnect storm and
  potentially lose far more). The JetStream duplicate window + the Go consumer's
  idempotency cover the at-least-once / at-most-once trade-off.

  The pool is a set of named Gnat connections (`:gnat_0`..`:gnat_{n-1}`) started
  by `Gnat.ConnectionSupervisor` in the application supervisor. This GenServer
  round-robins publishes across them via an atomic counter.
  """

  use GenServer

  require Logger

  alias Gnat.Jetstream.API.Stream

  @telemetry_publish [:dia_gateway, :publish]
  @telemetry_stream [:dia_gateway, :stream]

  # ── Public API ─────────────────────────────────────────────────────────────

  def start_link(opts) do
    GenServer.start_link(__MODULE__, opts, name: __MODULE__)
  end

  @doc """
  Publish one event envelope.

  `subject` is the fully-qualified NATS subject, `payload` is the iodata/binary
  JSON body, and `dedup_id` is the natural id used as the `Nats-Msg-Id` header.

  Always returns `:ok` from the caller's perspective — failures are handled
  internally (retry then drop) so the shard pipeline is never affected. Runs in
  the caller's process (the consumer task) so publishing does not serialize
  through this GenServer.
  """
  @spec publish(String.t(), iodata(), String.t()) :: :ok
  def publish(subject, payload, dedup_id) do
    cfg = config()
    retries = Keyword.fetch!(cfg, :publish_retries)
    conn = next_conn(Keyword.fetch!(cfg, :pool_size))

    do_publish(conn, subject, payload, dedup_id, retries)
  end

  # ── Helpers shared with the application supervisor ──────────────────────────

  @doc "Registered name for pool slot `i`."
  @spec conn_name(non_neg_integer()) :: atom()
  def conn_name(i), do: :"gnat_#{i}"

  @doc """
  Parse a `nats://host:port` (or `host:port`) URL into `{host, port}`.

  Only a single host is supported here; for multi-host failover, run more pool
  slots or extend `connection_settings` in the application supervisor.
  """
  @spec parse_url(String.t()) :: {String.t(), pos_integer()}
  def parse_url(url) do
    stripped =
      url
      |> String.trim()
      |> String.replace_prefix("nats://", "")
      |> String.replace_prefix("tls://", "")

    case String.split(stripped, ":", parts: 2) do
      [host, port] -> {host, String.to_integer(port)}
      [host] -> {host, 4222}
    end
  end

  # ── GenServer ──────────────────────────────────────────────────────────────

  @impl true
  def init(_opts) do
    # Initialise the round-robin counter shared by all caller processes.
    :persistent_term.put({__MODULE__, :rr}, :atomics.new(1, signed: false))
    # Ensure the stream once we (probably) have a live connection. Don't block
    # boot on NATS being up — retry asynchronously.
    send(self(), :ensure_stream)
    {:ok, %{stream_ready: false}}
  end

  @impl true
  def handle_info(:ensure_stream, state) do
    case ensure_stream() do
      :ok ->
        :telemetry.execute(@telemetry_stream, %{count: 1}, %{result: :ok})
        Logger.info("jetstream stream ensured")
        {:noreply, %{state | stream_ready: true}}

      {:error, reason} ->
        :telemetry.execute(@telemetry_stream, %{count: 1}, %{result: :error})

        Logger.warning(
          "could not ensure jetstream stream (#{inspect(reason)}); retrying in 2s"
        )

        Process.send_after(self(), :ensure_stream, 2000)
        {:noreply, state}
    end
  end

  # ── Stream creation ─────────────────────────────────────────────────────────

  defp ensure_stream do
    cfg = config()
    pool_size = Keyword.fetch!(cfg, :pool_size)
    conn = conn_name(0)

    # Wait until at least one pooled connection is actually up; otherwise the
    # request below would raise (no registered process yet).
    if Process.whereis(conn) do
      stream = %Stream{
        name: Keyword.fetch!(cfg, :stream_name),
        subjects: Keyword.fetch!(cfg, :stream_subjects),
        storage: :file,
        retention: :limits,
        discard: :old,
        max_age: to_nanoseconds(Keyword.fetch!(cfg, :max_age_seconds)),
        duplicate_window: to_nanoseconds(Keyword.fetch!(cfg, :duplicate_window_seconds))
      }

      case Stream.create(conn, stream) do
        {:ok, _info} ->
          :ok

        {:error, %{"err_code" => 10_058}} ->
          # "stream name already in use" — idempotent success.
          :ok

        {:error, %{"description" => desc}} when is_binary(desc) ->
          if already_exists?(desc), do: :ok, else: {:error, desc}

        {:error, reason} ->
          if already_exists?(reason), do: :ok, else: {:error, reason}
      end
    else
      {:error, {:no_connection, conn, pool_size}}
    end
  rescue
    e -> {:error, e}
  catch
    :exit, reason -> {:error, {:exit, reason}}
  end

  defp already_exists?(reason) do
    str = if is_binary(reason), do: reason, else: inspect(reason)
    String.contains?(String.downcase(str), ["already in use", "already exists"])
  end

  # ── Publishing ───────────────────────────────────────────────────────────────

  defp do_publish(_conn, subject, _payload, dedup_id, attempts_left) when attempts_left <= 0 do
    :telemetry.execute(@telemetry_publish, %{count: 1}, %{result: :dropped, subject: subject})
    Logger.warning("dropping event after exhausting retries: subject=#{subject} id=#{dedup_id}")
    :ok
  end

  defp do_publish(conn, subject, payload, dedup_id, attempts_left) do
    headers = if dedup_id in [nil, ""], do: [], else: [{"Nats-Msg-Id", dedup_id}]

    # JetStream publishes are request/reply: the server answers with a PubAck on
    # success, allowing us to confirm persistence and retry on failure.
    result =
      try do
        Gnat.request(conn, subject, payload, headers: headers, receive_timeout: 5000)
      rescue
        e -> {:error, e}
      catch
        :exit, reason -> {:error, {:exit, reason}}
      end

    case result do
      {:ok, %{body: body}} ->
        if pub_ack?(body) do
          :telemetry.execute(@telemetry_publish, %{count: 1}, %{result: :ok, subject: subject})
          :ok
        else
          retry(conn, subject, payload, dedup_id, attempts_left, {:nak, body})
        end

      {:error, reason} ->
        retry(conn, subject, payload, dedup_id, attempts_left, reason)
    end
  end

  defp retry(conn, subject, payload, dedup_id, attempts_left, reason) do
    :telemetry.execute(@telemetry_publish, %{count: 1}, %{result: :retry, subject: subject})

    Logger.debug(
      "publish attempt failed (#{inspect(reason)}); #{attempts_left - 1} retries left: #{subject}"
    )

    do_publish(conn, subject, payload, dedup_id, attempts_left - 1)
  end

  # A JetStream PubAck looks like {"stream":"...","seq":N}. An error looks like
  # {"error":{"code":...,"description":"..."}}.
  defp pub_ack?(body) when is_binary(body) do
    case Jason.decode(body) do
      {:ok, %{"stream" => _}} -> true
      _ -> false
    end
  end

  defp pub_ack?(_), do: false

  # ── Pool round-robin ─────────────────────────────────────────────────────────

  defp next_conn(pool_size) do
    ref = :persistent_term.get({__MODULE__, :rr})
    n = :atomics.add_get(ref, 1, 1)
    conn_name(rem(n, pool_size))
  end

  defp config, do: Application.fetch_env!(:dia_gateway, __MODULE__)

  defp to_nanoseconds(seconds), do: seconds * 1_000_000_000
end
