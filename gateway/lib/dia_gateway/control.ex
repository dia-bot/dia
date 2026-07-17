defmodule Dia.Gateway.Control do
  @moduledoc """
  The custom-bot control plane: the gateway's only inbound channel.

  Customers can run their own Discord application ("bring your own token") on our
  infrastructure. The Go services own the credentials and the desired state; this
  GenServer is the executor on the gateway side. It:

    * subscribes to the core-NATS subject `dia.control.bots` and, per command,
      starts / stops / restyles a customer bot as a `Nostrum.Bot` child under the
      `Dia.Gateway.BotSupervisor` DynamicSupervisor;
    * applies each bot's presence (status + activity) once it is READY — the one
      thing the shared bot can't do per guild, made possible here because every
      custom bot has its own gateway connection;
    * reports connection state back on `dia.control.gateway` so the dashboard can
      show "ready" / "error", and announces a `ready` hello on boot so the Go
      side replays the full desired set (the control plane is latest-wins, not a
      durable log, so a missed message self-heals on the next reconcile).

  The contract (subjects + JSON shapes) is defined in Go at
  `internal/event/control.go`; keep the two in lockstep.
  """

  use GenServer

  require Logger

  @command_subject "dia.control.bots"
  @status_subject "dia.control.gateway"

  # ── Public API ─────────────────────────────────────────────────────────────

  def start_link(opts), do: GenServer.start_link(__MODULE__, opts, name: __MODULE__)

  @doc """
  Called by the consumer when a bot finishes IDENTIFY. For a tracked custom bot
  this flips its dashboard state to `ready` and (re)applies its presence; the
  platform bot (and any untracked id) is ignored.
  """
  def on_ready(app_id) when is_binary(app_id) and app_id != "" do
    GenServer.cast(__MODULE__, {:ready, app_id})
  end

  def on_ready(_), do: :ok

  # ── GenServer ──────────────────────────────────────────────────────────────

  @impl true
  def init(_opts) do
    # children:  app_id (string) => Nostrum.Bot child pid
    # presences: app_id (string) => presence map (or nil) — also the "is tracked" set
    # monitors:  monitor ref      => app_id
    send(self(), :subscribe)
    {:ok, %{sub: nil, children: %{}, presences: %{}, monitors: %{}}}
  end

  @impl true
  def handle_info(:subscribe, state) do
    conn = conn()

    if Process.whereis(conn) do
      case Gnat.sub(conn, self(), @command_subject) do
        {:ok, sub} ->
          Logger.info("control plane subscribed on #{@command_subject}")
          # Announce ourselves so the Go side replays every enabled bot.
          publish_status(%{"event" => "ready"})
          {:noreply, %{state | sub: sub}}

        {:error, reason} ->
          Logger.warning("control subscribe failed (#{inspect(reason)}); retrying")
          Process.send_after(self(), :subscribe, 2000)
          {:noreply, state}
      end
    else
      Process.send_after(self(), :subscribe, 1000)
      {:noreply, state}
    end
  end

  # A control command arrived on dia.control.bots.
  def handle_info({:msg, %{body: body}}, state) do
    case Jason.decode(body) do
      {:ok, cmd} ->
        {:noreply, handle_command(cmd, state)}

      {:error, reason} ->
        Logger.warning("control: bad command json (#{inspect(reason)})")
        {:noreply, state}
    end
  end

  # A custom bot went down (bad token, disallowed intents, revoked bot). It was
  # started :temporary so the DynamicSupervisor won't loop-restart it; report the
  # failure and let the Go side decide whether to re-ensure.
  def handle_info({:DOWN, ref, :process, _pid, reason}, state) do
    case Map.pop(state.monitors, ref) do
      {nil, _} ->
        {:noreply, state}

      {app_id, monitors} ->
        Logger.warning("custom bot #{app_id} down: #{inspect(reason)}")
        report(app_id, "error", describe(reason))

        {:noreply,
         %{
           state
           | monitors: monitors,
             children: Map.delete(state.children, app_id),
             presences: Map.delete(state.presences, app_id)
         }}
    end
  end

  def handle_info(_other, state), do: {:noreply, state}

  @impl true
  def handle_cast({:ready, app_id}, state) do
    if Map.has_key?(state.presences, app_id) do
      report(app_id, "ready", "")
      apply_presence(app_id, state.presences[app_id])
    end

    {:noreply, state}
  end

  # ── Command handlers ───────────────────────────────────────────────────────

  defp handle_command(%{"action" => "ensure", "app_id" => app_id} = cmd, state)
       when is_binary(app_id) and app_id != "" do
    presence = cmd["presence"]

    if Map.has_key?(state.children, app_id) do
      # Already running: just refresh the desired presence and (re)apply it.
      apply_presence(app_id, presence)
      %{state | presences: Map.put(state.presences, app_id, presence)}
    else
      start_bot(app_id, cmd["token"], presence, state)
    end
  end

  defp handle_command(%{"action" => "presence", "app_id" => app_id} = cmd, state)
       when is_binary(app_id) do
    presence = cmd["presence"]
    apply_presence(app_id, presence)
    %{state | presences: Map.put(state.presences, app_id, presence)}
  end

  defp handle_command(%{"action" => "remove", "app_id" => app_id}, state)
       when is_binary(app_id) do
    stop_bot(app_id, state)
  end

  defp handle_command(cmd, state) do
    Logger.warning("control: ignoring command #{inspect(cmd)}")
    state
  end

  # ── Bot lifecycle ──────────────────────────────────────────────────────────

  defp start_bot(_app_id, token, _presence, state) when token in [nil, ""], do: state

  defp start_bot(app_id, token, presence, state) do
    report(app_id, "connecting", "")

    bot_options = %{
      name: bot_name(app_id),
      consumer: Dia.Gateway.Consumer,
      intents: custom_bot_intents(),
      wrapped_token: fn -> token end,
      shards: :auto,
      request_guild_members: false
    }

    # :temporary — a crash (bad token, disallowed intents) must NOT loop-restart;
    # we monitor for :DOWN, report the error, and wait for the next ensure.
    spec = Supervisor.child_spec({Nostrum.Bot, bot_options}, restart: :temporary)

    case DynamicSupervisor.start_child(Dia.Gateway.Application.bot_supervisor(), spec) do
      {:ok, pid} ->
        ref = Process.monitor(pid)

        %{
          state
          | children: Map.put(state.children, app_id, pid),
            presences: Map.put(state.presences, app_id, presence),
            monitors: Map.put(state.monitors, ref, app_id)
        }

      {:error, reason} ->
        Logger.warning("failed to start custom bot #{app_id}: #{inspect(reason)}")
        report(app_id, "error", describe(reason))
        state
    end
  end

  defp stop_bot(app_id, state) do
    case Map.pop(state.children, app_id) do
      {nil, _} ->
        report(app_id, "disconnected", "")
        state

      {pid, children} ->
        DynamicSupervisor.terminate_child(Dia.Gateway.Application.bot_supervisor(), pid)
        report(app_id, "disconnected", "")

        monitors =
          state.monitors
          |> Enum.reject(fn {_ref, id} -> id == app_id end)
          |> Map.new()

        %{
          state
          | children: children,
            presences: Map.delete(state.presences, app_id),
            monitors: monitors
        }
    end
  end

  # ── Presence ───────────────────────────────────────────────────────────────

  defp apply_presence(_app_id, nil), do: :ok

  defp apply_presence(app_id, presence) do
    status = status_atom(presence["status"])
    activity = activity_tuple(presence)

    Nostrum.Bot.with_bot(bot_name(app_id), fn ->
      Nostrum.Api.Self.update_status(status, activity)
    end)

    :ok
  rescue
    e -> Logger.warning("presence apply failed for #{app_id}: #{inspect(e)}")
  catch
    :exit, reason -> Logger.warning("presence apply exit for #{app_id}: #{inspect(reason)}")
  end

  defp status_atom("idle"), do: :idle
  defp status_atom("dnd"), do: :dnd
  defp status_atom("invisible"), do: :invisible
  defp status_atom(_), do: :online

  # Map our presence into Nostrum's activity tuple. No activity (or blank text)
  # becomes an empty custom status so the status dot still applies.
  defp activity_tuple(%{"activity_type" => t, "activity_text" => text} = p)
       when is_integer(t) and t >= 0 do
    text = to_string(text)

    case t do
      0 -> {:playing, text}
      1 -> {:streaming, text, to_string(p["activity_url"] || "")}
      2 -> {:listening, text}
      3 -> {:watching, text}
      5 -> {:competing, text}
      _ -> {:custom, text}
    end
  end

  defp activity_tuple(_), do: {:custom, ""}

  # ── NATS helpers ───────────────────────────────────────────────────────────

  defp report(app_id, state_str, error) do
    publish_status(%{
      "event" => "bot_state",
      "app_id" => app_id,
      "state" => state_str,
      "error" => error
    })
  end

  defp publish_status(map) do
    case Jason.encode(map) do
      {:ok, body} ->
        conn = conn()
        if Process.whereis(conn), do: Gnat.pub(conn, @status_subject, body), else: :ok

      {:error, _} ->
        :ok
    end
  rescue
    _ -> :ok
  catch
    _, _ -> :ok
  end

  defp conn, do: Dia.Gateway.Publisher.conn_name(0)

  defp custom_bot_intents, do: Application.fetch_env!(:dia_gateway, :custom_bot_intents)

  # Nostrum bot name = the integer application id (unique per bot). fetch_bot_name
  # in the consumer returns it, and we stamp it as the envelope app_id.
  defp bot_name(app_id), do: String.to_integer(app_id)

  defp describe(reason) when is_binary(reason), do: reason
  defp describe(reason), do: inspect(reason)
end
