defmodule Dia.Gateway.Consumer do
  @moduledoc """
  The shared Nostrum consumer for every bot the gateway runs (the platform bot
  and each customer's custom bot). In Nostrum 0.11 the consumer is a plain
  `@behaviour Nostrum.Consumer` module named by each bot's `:consumer` option,
  invoked per event with that bot's context; `current_app_id/0` reads that
  context so each envelope is stamped with the producing bot's application id.

  Nostrum runs each event in its own `Task` (free parallelism + isolation). We
  implement one `handle_event/1` clause per forwarded event type that:

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

  @behaviour Nostrum.Consumer

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

  # ── Threads (threads are channels) ───────────────────────────────────────────────

  def handle_event({:THREAD_CREATE, thread, ws}) when is_map(thread) do
    forward(:THREAD_CREATE, guild_id(thread), ws, Mapper.map_thread(thread), id_of(thread))
  end

  def handle_event({:THREAD_DELETE, :noop, _ws}) do
    Logger.debug("THREAD_DELETE dropped: thread not recoverable under NoOp cache")
    :ok
  end

  def handle_event({:THREAD_DELETE, thread, ws}) when is_map(thread) do
    forward(:THREAD_DELETE, guild_id(thread), ws, Mapper.map_thread(thread), id_of(thread))
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

  # GUILD_MEMBER_UPDATE delivered as {guild_id, old_member, new_member}. We pass
  # the old member through so the mapper can emit old_roles (when the cache has
  # it) for added/removed-role and boost detection downstream.
  def handle_event({:GUILD_MEMBER_UPDATE, {gid, old, member}, ws}) do
    data = Mapper.map_guild_member_update(gid, member, old)
    forward(:GUILD_MEMBER_UPDATE, gid, ws, data, member_dedup(gid, member))
  end

  # ── Bans ───────────────────────────────────────────────────────────────────────

  # GUILD_BAN_ADD / GUILD_BAN_REMOVE deliver a struct with guild_id + user.
  def handle_event({:GUILD_BAN_ADD, ban, ws}) do
    gid = guild_id(ban)
    data = Mapper.map_ban(gid, Map.get(ban, :user))
    forward(:GUILD_BAN_ADD, gid, ws, data, ban_dedup(gid, ban))
  end

  def handle_event({:GUILD_BAN_REMOVE, ban, ws}) do
    gid = guild_id(ban)
    data = Mapper.map_ban(gid, Map.get(ban, :user))
    forward(:GUILD_BAN_REMOVE, gid, ws, data, ban_dedup(gid, ban))
  end

  # ── Messages ──────────────────────────────────────────────────────────────────

  def handle_event({:MESSAGE_CREATE, message, ws}) do
    forward(:MESSAGE_CREATE, guild_id(message), ws, Mapper.map_message(message), id_of(message))
  end

  # MESSAGE_UPDATE arrives as {old, new} when the message cache is on, or as a
  # bare (possibly partial) message under the thin cache. Handle both. A partial
  # update with no content still carries ids; the worker tolerates empty fields.
  def handle_event({:MESSAGE_UPDATE, {_old, message}, ws}) do
    forward(
      :MESSAGE_UPDATE,
      guild_id(message),
      ws,
      Mapper.map_message_update(message),
      id_of(message)
    )
  end

  def handle_event({:MESSAGE_UPDATE, message, ws}) when is_map(message) do
    forward(
      :MESSAGE_UPDATE,
      guild_id(message),
      ws,
      Mapper.map_message_update(message),
      id_of(message)
    )
  end

  # MESSAGE_DELETE carries the deleted message's ids (guild_id may be nil in DMs).
  def handle_event({:MESSAGE_DELETE, :noop, _ws}) do
    Logger.debug("MESSAGE_DELETE dropped: message not recoverable under NoOp cache")
    :ok
  end

  def handle_event({:MESSAGE_DELETE, message, ws}) when is_map(message) do
    forward(
      :MESSAGE_DELETE,
      guild_id(message),
      ws,
      Mapper.map_message_delete(message),
      id_of(message)
    )
  end

  # ── Reactions ──────────────────────────────────────────────────────────────────

  def handle_event({:MESSAGE_REACTION_ADD, reaction, ws}) do
    gid = guild_id(reaction)
    data = Mapper.map_reaction(reaction)
    forward(:MESSAGE_REACTION_ADD, gid, ws, data, reaction_dedup(:add, reaction))
  end

  def handle_event({:MESSAGE_REACTION_REMOVE, reaction, ws}) do
    gid = guild_id(reaction)
    data = Mapper.map_reaction(reaction)
    forward(:MESSAGE_REACTION_REMOVE, gid, ws, data, reaction_dedup(:remove, reaction))
  end

  # ── Voice ──────────────────────────────────────────────────────────────────────

  # VOICE_STATE_UPDATE arrives as {old, new} with a voice cache, or a bare voice
  # state under the thin cache. channel_id nil on the new state == disconnect.
  def handle_event({:VOICE_STATE_UPDATE, {_old, vs}, ws}) do
    forward(:VOICE_STATE_UPDATE, guild_id(vs), ws, Mapper.map_voice_state(vs), voice_dedup(vs))
  end

  def handle_event({:VOICE_STATE_UPDATE, vs, ws}) when is_map(vs) do
    forward(:VOICE_STATE_UPDATE, guild_id(vs), ws, Mapper.map_voice_state(vs), voice_dedup(vs))
  end

  # ── Interactions ──────────────────────────────────────────────────────────────

  def handle_event({:INTERACTION_CREATE, interaction, ws}) do
    gid = guild_id(interaction)
    data = Mapper.map_interaction(interaction)
    forward(:INTERACTION_CREATE, gid, ws, data, id_of(interaction))
  end

  # ── Readiness ─────────────────────────────────────────────────────────────

  # A bot finishing IDENTIFY tells the control plane it is live, so the Go side
  # can flip the custom bot's dashboard state to "ready" and (re)apply its
  # configured presence. The platform bot readies too; Control ignores app ids
  # it isn't tracking.
  def handle_event({:READY, _data, _ws}) do
    Dia.Gateway.Control.on_ready(current_app_id())
    :ok
  end

  # Everything else (TYPING_START, presence, etc.) is dropped by this explicit
  # catch-all (0.11 no longer injects one via `use Nostrum.Consumer`).
  def handle_event(_), do: :ok

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
      "data" => data,
      # Which bot produced this event. For the platform bot this is its own id
      # (the Go side treats the platform id, or "", as the shared bot); for a
      # customer's bot it is their application id, so the worker picks the right
      # token to act and respond with.
      "app_id" => current_app_id()
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

  # The current bot's Nostrum name as a string. In multi-bot mode the event task
  # runs with its bot's context, so fetch_bot_name/0 resolves to that bot; name
  # defaults to the integer bot/application id. Guarded so a lookup miss can
  # never crash the pipeline (falls back to "").
  defp current_app_id do
    to_string(Nostrum.Bot.fetch_bot_name())
  rescue
    _ -> ""
  catch
    _, _ -> ""
  end

  defp guild_id(%{guild_id: gid}), do: gid
  defp guild_id(_), do: nil

  defp id_of(%{id: id}), do: id
  defp id_of(_), do: nil

  # Members have no stable single id; derive a dedupe id from guild + user.
  defp member_dedup(gid, member) do
    uid = Map.get(member, :user_id)
    "#{gid}-#{uid}"
  end

  # Bans / reactions / voice states have no single id either; derive stable
  # dedupe ids from their natural composite keys.
  defp ban_dedup(gid, ban) do
    uid = id_of(Map.get(ban, :user)) || Map.get(ban, :user_id)
    "#{gid}-#{uid}"
  end

  defp reaction_dedup(dir, r) do
    "#{dir}-#{Map.get(r, :message_id)}-#{Map.get(r, :user_id)}-#{emoji_key(Map.get(r, :emoji))}"
  end

  defp emoji_key(nil), do: ""
  defp emoji_key(e), do: to_string(Map.get(e, :id) || Map.get(e, :name))

  # A member's voice state changes many times; key on guild+user+session+channel
  # plus the millisecond so distinct transitions don't dedupe each other away.
  defp voice_dedup(vs) do
    "#{Map.get(vs, :guild_id)}-#{Map.get(vs, :user_id)}-#{Map.get(vs, :channel_id)}-#{System.system_time(:millisecond)}"
  end

  # When the natural id is unrecoverable, derive a stable-ish fallback so two
  # identical redeliveries within the window still dedupe. Uses shard + ms.
  defp dedup_fallback(event_name, ws) do
    "#{event_name}-#{shard_id(ws)}-#{System.system_time(:millisecond)}"
  end
end
