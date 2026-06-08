defmodule Dia.Gateway.Consumer do
  @moduledoc """
  The single Nostrum consumer for the gateway.

  `use Nostrum.Consumer` gives us `start_link/1`, `child_spec/1` and a default
  `handle_event/1` catch-all, and runs each event in its own `Task` (free
  parallelism + isolation). We implement one catch-all `handle_event/1` clause
  per forwarded event type that:

    1. maps the Nostrum struct(s) into the normalized contract via
       `Dia.Gateway.Mapper`,
    2. wraps the result in an `Envelope`
       (`{type, guild_id, shard_id, ts, data}`),
    3. publishes it to `discord.events.<TYPE>.<guild_id>` with a dedupe id.

  `TYPE` is the upper-case Discord event name, i.e. `Atom.to_string(event_name)`.
  Any event type not in the forward list is silently ignored.

  Publishing never raises into the shard pipeline: `Publisher.publish/3`
  swallows NATS failures (retry then drop).
  """

  use Nostrum.Consumer

  require Logger

  alias Dia.Gateway.{Mapper, Publisher}

  @subject_prefix "discord.events"

  # ── Guild lifecycle ──────────────────────────────────────────────────────────

  def handle_event({:GUILD_CREATE, guild, ws}) do
    forward(:GUILD_CREATE, guild_id(guild), ws, Mapper.map_guild(guild), id_of(guild))
  end

  # A guild returning from "unavailable" (e.g. after an outage) is delivered by
  # Nostrum as GUILD_AVAILABLE, but it carries the same full guild snapshot as
  # GUILD_CREATE. The Go contract has no GUILD_AVAILABLE type, so we forward it
  # AS a GUILD_CREATE so the worker still gets the refreshed guild state.
  def handle_event({:GUILD_AVAILABLE, guild, ws}) do
    forward(:GUILD_CREATE, guild_id(guild), ws, Mapper.map_guild(guild), id_of(guild))
  end

  # GUILD_UPDATE is delivered as {old_guild, new_guild} (old is nil under NoOp).
  def handle_event({:GUILD_UPDATE, {_old, guild}, ws}) do
    forward(:GUILD_UPDATE, guild_id(guild), ws, Mapper.map_guild(guild), id_of(guild))
  end

  # GUILD_DELETE is delivered as {old_guild, unavailable}. Under the NoOp guild
  # cache `old_guild` is `nil`, so the guild id is NOT recoverable here. We still
  # forward an (id-less) GuildDelete so downstream sees the unavailability bit;
  # the subject guild segment falls back to "0". If a worker needs the id on
  # delete, enable a real guild cache. (Documented limitation.)
  def handle_event({:GUILD_DELETE, {old_guild, unavailable}, ws}) do
    gid = if is_map(old_guild), do: id_of(old_guild), else: nil
    data = Mapper.map_guild_delete(%{id: gid, unavailable: unavailable})
    forward(:GUILD_DELETE, gid, ws, data, gid || dedup_fallback(:GUILD_DELETE, ws))
  end

  # A guild going unavailable (outage, not a real removal) is delivered as
  # GUILD_UNAVAILABLE carrying an UnavailableGuild struct WITH its id intact.
  # We forward it as a GUILD_DELETE with unavailable=true — and unlike the NoOp
  # GUILD_DELETE path above, the guild id is preserved here.
  def handle_event({:GUILD_UNAVAILABLE, unavailable_guild, ws}) do
    gid = id_of(unavailable_guild)
    data = Mapper.map_guild_delete(%{id: gid, unavailable: true})
    forward(:GUILD_DELETE, gid, ws, data, gid || dedup_fallback(:GUILD_DELETE, ws))
  end

  # ── Channels ───────────────────────────────────────────────────────────────────

  def handle_event({:CHANNEL_CREATE, channel, ws}) do
    forward(:CHANNEL_CREATE, guild_id(channel), ws, Mapper.map_channel(channel), id_of(channel))
  end

  # CHANNEL_UPDATE is delivered as {old, new} ({channel, channel} under NoOp).
  def handle_event({:CHANNEL_UPDATE, {_old, channel}, ws}) do
    forward(:CHANNEL_UPDATE, guild_id(channel), ws, Mapper.map_channel(channel), id_of(channel))
  end

  def handle_event({:CHANNEL_UPDATE, channel, ws}) when is_map(channel) do
    forward(:CHANNEL_UPDATE, guild_id(channel), ws, Mapper.map_channel(channel), id_of(channel))
  end

  # CHANNEL_DELETE under the NoOp guild cache arrives as :noop, losing the
  # channel object entirely (id, guild). It is therefore not forwardable.
  # (Documented limitation — enable a guild cache to recover deletes.)
  def handle_event({:CHANNEL_DELETE, :noop, _ws}) do
    Logger.debug("CHANNEL_DELETE dropped: channel not recoverable under NoOp cache")
    :ok
  end

  def handle_event({:CHANNEL_DELETE, channel, ws}) when is_map(channel) do
    forward(:CHANNEL_DELETE, guild_id(channel), ws, Mapper.map_channel(channel), id_of(channel))
  end

  # ── Roles ───────────────────────────────────────────────────────────────────────

  # GUILD_ROLE_CREATE delivered as {guild_id, new_role}.
  def handle_event({:GUILD_ROLE_CREATE, {gid, role}, ws}) do
    data = Mapper.map_role_event(gid, role)
    forward(:GUILD_ROLE_CREATE, gid, ws, data, id_of(role))
  end

  # GUILD_ROLE_UPDATE delivered as {guild_id, old_role, new_role}.
  def handle_event({:GUILD_ROLE_UPDATE, {gid, _old, role}, ws}) do
    data = Mapper.map_role_event(gid, role)
    forward(:GUILD_ROLE_UPDATE, gid, ws, data, id_of(role))
  end

  # GUILD_ROLE_DELETE under NoOp arrives as :noop (GuildCache.role_delete). The
  # role id and guild id are not recoverable. (Documented limitation.)
  def handle_event({:GUILD_ROLE_DELETE, :noop, _ws}) do
    Logger.debug("GUILD_ROLE_DELETE dropped: role not recoverable under NoOp cache")
    :ok
  end

  # If a real cache is configured, delete arrives as {guild_id, old_role}.
  def handle_event({:GUILD_ROLE_DELETE, {gid, role}, ws}) do
    role_id = id_of(role)
    data = Mapper.map_role_delete(gid, role_id)
    forward(:GUILD_ROLE_DELETE, gid, ws, data, role_id || dedup_fallback(:GUILD_ROLE_DELETE, ws))
  end

  # ── Members ───────────────────────────────────────────────────────────────────

  # GUILD_MEMBER_ADD delivered as {guild_id, member}.
  def handle_event({:GUILD_MEMBER_ADD, {gid, member}, ws}) do
    data = Mapper.map_guild_member_add(gid, member)
    forward(:GUILD_MEMBER_ADD, gid, ws, data, member_dedup(gid, member))
  end

  # GUILD_MEMBER_REMOVE under the NoOp member cache arrives as :noop, losing the
  # user and guild id. Not forwardable. (Documented limitation — enable a member
  # cache to recover removals.)
  def handle_event({:GUILD_MEMBER_REMOVE, :noop, _ws}) do
    Logger.debug("GUILD_MEMBER_REMOVE dropped: member not recoverable under NoOp cache")
    :ok
  end

  # With a real cache: {guild_id, old_member}. We can at least emit the user id.
  def handle_event({:GUILD_MEMBER_REMOVE, {gid, member}, ws}) when is_map(member) do
    user_map = %{"id" => Mapper.id(Map.get(member, :user_id))}
    data = Mapper.map_guild_member_remove(gid, user_map)
    forward(:GUILD_MEMBER_REMOVE, gid, ws, data, member_dedup(gid, member))
  end

  def handle_event({:GUILD_MEMBER_REMOVE, member, ws}) when is_map(member) do
    gid = Map.get(member, :guild_id)
    user_map = %{"id" => Mapper.id(Map.get(member, :user_id))}
    data = Mapper.map_guild_member_remove(gid, user_map)
    forward(:GUILD_MEMBER_REMOVE, gid, ws, data, member_dedup(gid, member))
  end

  # GUILD_MEMBER_UPDATE delivered as {guild_id, old_member, new_member}.
  def handle_event({:GUILD_MEMBER_UPDATE, {gid, _old, member}, ws}) do
    data = Mapper.map_guild_member_update(gid, member)
    forward(:GUILD_MEMBER_UPDATE, gid, ws, data, member_dedup(gid, member))
  end

  # ── Messages ──────────────────────────────────────────────────────────────────

  def handle_event({:MESSAGE_CREATE, message, ws}) do
    forward(:MESSAGE_CREATE, guild_id(message), ws, Mapper.map_message(message), id_of(message))
  end

  # ── Interactions ──────────────────────────────────────────────────────────────

  def handle_event({:INTERACTION_CREATE, interaction, ws}) do
    gid = guild_id(interaction)
    data = Mapper.map_interaction(interaction)
    forward(:INTERACTION_CREATE, gid, ws, data, id_of(interaction))
  end

  # Everything else (READY, TYPING_START, presence, etc.) is ignored by the
  # `handle_event(_)` catch-all that `use Nostrum.Consumer` injects via
  # `__before_compile__`, so we deliberately do not define our own — a redundant
  # catch-all here would shadow Nostrum's and trip the compiler's clause check.

  # ── Internal ──────────────────────────────────────────────────────────────────

  # Build the envelope, encode, and publish to the subject. `dedup_id` becomes
  # the Nats-Msg-Id header for JetStream dedupe within the duplicate window.
  defp forward(event_name, guild_id, ws, data, dedup_id) do
    type = Atom.to_string(event_name)
    gid_str = guild_segment(guild_id)
    shard_id = shard_id(ws)

    envelope = %{
      "type" => type,
      "guild_id" => guild_id_field(guild_id),
      "shard_id" => shard_id,
      "ts" => System.system_time(:millisecond),
      "data" => data
    }

    subject = @subject_prefix <> "." <> type <> "." <> gid_str

    case Jason.encode(envelope) do
      {:ok, body} ->
        Publisher.publish(subject, body, dedup_key(type, dedup_id))

      {:error, reason} ->
        Logger.error("failed to encode #{type} envelope: #{inspect(reason)}")
        :ok
    end
  end

  # JetStream dedupe is keyed on the `Nats-Msg-Id` header per STREAM (not per
  # subject), so the id MUST be namespaced by event type. Several types share the
  # bare guild id as their natural dedupe id — GUILD_CREATE, GUILD_UPDATE,
  # GUILD_DELETE, and the READY-time GUILD_UNAVAILABLE (forwarded as GUILD_DELETE)
  # — and on connect Discord delivers the unavailable GUILD_DELETE burst *before*
  # the GUILD_CREATE snapshot. Without the type prefix the later GUILD_CREATE
  # collides with that just-published GUILD_DELETE inside the duplicate window and
  # is silently dropped, so the guild never reaches the worker. Prefixing keeps
  # per-type idempotency (a redelivered GUILD_CREATE still dedupes) while letting
  # create/update/delete for one guild coexist.
  defp dedup_key(_type, nil), do: ""
  defp dedup_key(_type, ""), do: ""
  defp dedup_key(type, dedup_id), do: type <> ":" <> to_string(dedup_id)

  # Subject guild segment: empty / nil guild id becomes the "0" token.
  defp guild_segment(nil), do: "0"
  defp guild_segment(""), do: "0"
  defp guild_segment(gid) when is_integer(gid), do: Integer.to_string(gid)
  defp guild_segment(gid) when is_binary(gid), do: gid

  # Envelope `guild_id` field: empty string when absent (matches Go's "").
  defp guild_id_field(nil), do: ""
  defp guild_id_field(gid) when is_integer(gid), do: Integer.to_string(gid)
  defp guild_id_field(gid) when is_binary(gid), do: gid

  # WSState.shard_num is already the 0-based Discord shard id (Nostrum connects
  # ids (lowest-1)..(highest-1)), so no adjustment is needed.
  defp shard_id(%{shard_num: n}) when is_integer(n), do: n
  defp shard_id(_), do: 0

  defp guild_id(%{guild_id: gid}), do: gid
  defp guild_id(_), do: nil

  defp id_of(%{id: id}), do: id
  defp id_of(_), do: nil

  # Members have no stable single id; derive a dedupe id from guild + user.
  defp member_dedup(gid, member) do
    uid = Map.get(member, :user_id)
    "#{gid}-#{uid}"
  end

  # When the natural id is unrecoverable, derive a stable-ish fallback so two
  # identical redeliveries within the window still dedupe. Uses shard + ms.
  defp dedup_fallback(event_name, ws) do
    "#{event_name}-#{shard_id(ws)}-#{System.system_time(:millisecond)}"
  end
end
