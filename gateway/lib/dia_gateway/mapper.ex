defmodule Dia.Gateway.Mapper do
  @moduledoc """
  Pure functions that convert Nostrum structs into the normalized JSON payloads
  defined by the Go `internal/event` package (the single source of truth for the
  NATS contract).

  Hard rules carried over from the Go side:

    * **All snowflake IDs are decimal strings**, never integers. Nostrum stores
      them as integers, so every id goes through `id/1`.
    * Field names match the Go `json:"..."` tags exactly.
    * Optional fields (`omitempty` in Go) are dropped when empty via `compact/1`
      so the wire shape matches Go's `encoding/json` output.

  Each `map_*/1` function returns the **`data`** map for one event type. The
  consumer wraps it in the `Envelope`. There is one function per forwarded event
  type.

  ## Known limitations of running "thin" (NoOp caches)

  Nostrum casts gateway payloads into its structs *before* our consumer sees
  them, and several of those structs drop fields we would like:

    * `Nostrum.Struct.Guild.Member` keeps only `user_id`, not the full nested
      `user`. So for `GUILD_MEMBER_ADD` / `GUILD_MEMBER_UPDATE` the emitted
      `member.user` has only `id` populated (other user fields are
      `omitempty`). The worker can hydrate from REST / its own cache.
    * `GUILD_MEMBER_REMOVE` reaches us as `:noop` from `MemberCache.NoOp.delete`,
      losing the user and guild id entirely — see `map_guild_member_remove/3`.
    * `GUILD_DELETE` reaches us as `{nil, unavailable}` because
      `GuildCache.NoOp.delete` returns `nil`, losing the guild id — see
      `map_guild_delete/1`.
    * `CHANNEL_DELETE` reaches us as `:noop` from
      `GuildCache.NoOp.channel_delete`, losing the channel — see
      `map_channel_delete/2`.

  The consumer compensates for these where it can by reading the raw event
  tuple shape; the cases that genuinely cannot be recovered are documented at
  their call sites.
  """

  # ── ID + scalar helpers ──────────────────────────────────────────────────────

  @doc "Snowflake (or any id) -> decimal string. nil stays nil (dropped later)."
  @spec id(integer() | String.t() | nil) :: String.t() | nil
  def id(nil), do: nil
  def id(v) when is_integer(v), do: Integer.to_string(v)
  def id(v) when is_binary(v), do: v

  defp ids(nil), do: []
  defp ids(list) when is_list(list), do: Enum.map(list, &id/1)

  # Permissions arrive as an integer bitfield in Nostrum; Go wants a decimal
  # string. nil -> "0" to keep a stable, non-empty bitfield string.
  defp perms(nil), do: "0"
  defp perms(v) when is_integer(v), do: Integer.to_string(v)
  defp perms(v) when is_binary(v), do: v

  # Nostrum casts `joined_at` to a unix timestamp (seconds). Go's `joined_at` is
  # a string (Discord delivers ISO8601). Re-render as ISO8601 for fidelity.
  defp iso8601(nil), do: nil
  defp iso8601(%DateTime{} = dt), do: DateTime.to_iso8601(dt)

  defp iso8601(unix) when is_integer(unix) do
    case DateTime.from_unix(unix) do
      {:ok, dt} -> DateTime.to_iso8601(dt)
      _ -> nil
    end
  end

  defp iso8601(other) when is_binary(other), do: other

  # Drop nil values and empty strings so the JSON matches Go `omitempty`. Lists
  # and booleans are kept as-is (the Go structs only `omitempty` strings/ids and
  # a few scalars; lists like `roles` are always present).
  defp compact(map) do
    map
    |> Enum.reject(fn {_k, v} -> is_nil(v) end)
    |> Map.new()
  end

  # ── Shared sub-objects ───────────────────────────────────────────────────────

  @doc "Nostrum.Struct.User -> Go `User`."
  def user(nil), do: nil

  def user(%{id: uid} = u) do
    compact(%{
      "id" => id(uid),
      "username" => Map.get(u, :username),
      "global_name" => blank_to_nil(Map.get(u, :global_name)),
      "discriminator" => blank_to_nil(Map.get(u, :discriminator)),
      "avatar" => blank_to_nil(Map.get(u, :avatar)),
      "bot" => true_or_nil(Map.get(u, :bot))
    })
  end

  @doc """
  Build a Go `Member`. `member` is a Nostrum member struct (has `user_id`,
  `roles`, `joined_at`, ...). `user` is the resolved full user object (a Go-User
  map) when available, else we synthesize one from `user_id` alone.
  """
  def member(nil, _user), do: nil

  def member(m, user_map) do
    user_obj =
      cond do
        is_map(user_map) and map_size(user_map) > 0 -> user_map
        true -> compact(%{"id" => id(Map.get(m, :user_id))})
      end

    compact(%{
      "user" => user_obj,
      "nick" => blank_to_nil(Map.get(m, :nick)),
      "avatar" => blank_to_nil(Map.get(m, :avatar)),
      # roles is always present (a list) per the Go contract.
      "roles" => ids(Map.get(m, :roles)),
      "joined_at" => iso8601(Map.get(m, :joined_at)),
      "premium_since" => iso8601(Map.get(m, :premium_since)),
      "pending" => true_or_nil(Map.get(m, :pending))
    })
    |> Map.put_new("roles", [])
  end

  @doc "Nostrum.Struct.Channel -> Go `Channel`."
  def channel(nil), do: nil

  def channel(c) do
    compact(%{
      "id" => id(Map.get(c, :id)),
      "guild_id" => id(Map.get(c, :guild_id)),
      "name" => Map.get(c, :name),
      "type" => Map.get(c, :type) || 0,
      "position" => Map.get(c, :position) || 0,
      "parent_id" => id(Map.get(c, :parent_id)),
      "topic" => blank_to_nil(Map.get(c, :topic)),
      "nsfw" => true_or_nil(Map.get(c, :nsfw))
    })
    |> Map.put_new("name", "")
  end

  @doc "Nostrum.Struct.Guild.Role -> Go `Role`."
  def role(nil), do: nil

  def role(r) do
    compact(%{
      "id" => id(Map.get(r, :id)),
      "name" => Map.get(r, :name) || "",
      "color" => Map.get(r, :color) || 0,
      "position" => Map.get(r, :position) || 0,
      "permissions" => perms(Map.get(r, :permissions)),
      "hoist" => true_or_nil(Map.get(r, :hoist)),
      "managed" => true_or_nil(Map.get(r, :managed)),
      "mentionable" => true_or_nil(Map.get(r, :mentionable)),
      "icon" => blank_to_nil(Map.get(r, :icon))
    })
  end

  # ── Guild events ──────────────────────────────────────────────────────────────

  @doc "GUILD_CREATE / GUILD_UPDATE data -> Go `Guild`."
  def map_guild(g) do
    compact(%{
      "id" => id(Map.get(g, :id)),
      "name" => Map.get(g, :name) || "",
      "icon" => blank_to_nil(Map.get(g, :icon)),
      "owner_id" => id(Map.get(g, :owner_id)),
      "member_count" => Map.get(g, :member_count) || 0,
      "channels" => list_or_nil(collection_values(Map.get(g, :channels)), &channel/1),
      "roles" => list_or_nil(collection_values(Map.get(g, :roles)), &role/1),
      "unavailable" => true_or_nil(Map.get(g, :unavailable))
    })
    |> Map.put_new("owner_id", "")
  end

  @doc "GUILD_DELETE data -> Go `GuildDelete`."
  def map_guild_delete(%{id: gid} = g) do
    compact(%{
      "id" => id(gid),
      "unavailable" => true_or_nil(Map.get(g, :unavailable))
    })
    |> Map.put_new("id", "")
  end

  # ── Channel events ────────────────────────────────────────────────────────────

  @doc "CHANNEL_CREATE / CHANNEL_UPDATE data -> Go `ChannelEvent` (embeds Channel)."
  def map_channel(c), do: channel(c)

  # ── Role events ───────────────────────────────────────────────────────────────

  @doc "GUILD_ROLE_CREATE / GUILD_ROLE_UPDATE data -> Go `RoleEvent`."
  def map_role_event(guild_id, role_struct) do
    compact(%{
      "guild_id" => id(guild_id) || "",
      "role" => role(role_struct)
    })
    |> Map.put_new("guild_id", "")
  end

  @doc "GUILD_ROLE_DELETE data -> Go `RoleEvent` with only role_id set."
  def map_role_delete(guild_id, role_id) do
    compact(%{
      "guild_id" => id(guild_id) || "",
      "role_id" => id(role_id)
    })
    |> Map.put_new("guild_id", "")
  end

  # ── Member events ─────────────────────────────────────────────────────────────

  @doc "GUILD_MEMBER_ADD data -> Go `MemberAdd`."
  def map_guild_member_add(guild_id, member_struct, member_count \\ nil) do
    compact(%{
      "guild_id" => id(guild_id) || "",
      "member" => member(member_struct, nil),
      "member_count" => member_count
    })
    |> Map.put_new("guild_id", "")
  end

  @doc """
  GUILD_MEMBER_UPDATE data -> Go `MemberUpdate`.

  `old_member` is the member's prior state (may be nil under the thin cache);
  when present we emit its role set as `old_roles` so the worker can diff
  added/removed roles and detect boosts.
  """
  def map_guild_member_update(guild_id, member_struct, old_member \\ nil) do
    compact(%{
      "guild_id" => id(guild_id) || "",
      "member" => member(member_struct, nil),
      "old_roles" => old_roles(old_member)
    })
    |> Map.put_new("guild_id", "")
  end

  defp old_roles(nil), do: nil
  defp old_roles(m) when is_map(m), do: list_or_nil_ids(Map.get(m, :roles))

  # ── Bans ──────────────────────────────────────────────────────────────────────

  @doc "GUILD_BAN_ADD / GUILD_BAN_REMOVE data -> Go `BanEvent`."
  def map_ban(guild_id, user_struct) do
    compact(%{
      "guild_id" => id(guild_id) || "",
      "user" => user(user_struct)
    })
    |> Map.put_new("guild_id", "")
  end

  @doc """
  GUILD_MEMBER_REMOVE data -> Go `MemberRemove`.

  `user` is a Go-User map (may be just `%{"id" => ...}` if only the id survived).
  """
  def map_guild_member_remove(guild_id, user_map, member_count \\ nil) do
    compact(%{
      "guild_id" => id(guild_id) || "",
      "user" => user_map || %{},
      "member_count" => member_count
    })
    |> Map.put_new("guild_id", "")
  end

  # ── Message ────────────────────────────────────────────────────────────────────

  @doc "MESSAGE_CREATE data -> Go `Message`."
  def map_message(m) do
    compact(%{
      "id" => id(Map.get(m, :id)),
      "guild_id" => id(Map.get(m, :guild_id)) || "",
      "channel_id" => id(Map.get(m, :channel_id)) || "",
      "content" => Map.get(m, :content) || "",
      "author" => user(Map.get(m, :author)),
      "member" => message_member(Map.get(m, :member), Map.get(m, :author)),
      "mention_everyone" => true_or_nil(Map.get(m, :mention_everyone)),
      "mention_roles" => list_or_nil_ids(Map.get(m, :mention_roles)),
      "mentions" => list_or_nil(Map.get(m, :mentions), &user/1),
      "attachment_count" => attachment_count(Map.get(m, :attachments))
    })
    |> Map.put_new("guild_id", "")
    |> Map.put_new("channel_id", "")
    |> Map.put_new("content", "")
  end

  # The partial member embedded in a message has no user_id of its own (Discord
  # omits it because the author carries the user). Use the author as the member
  # user so `member.user` is populated.
  defp message_member(nil, _author), do: nil

  defp message_member(m, author) do
    member(m, user(author))
  end

  defp attachment_count(nil), do: nil
  defp attachment_count([]), do: nil
  defp attachment_count(list) when is_list(list), do: length(list)

  @doc "MESSAGE_UPDATE data -> Go `MessageUpdate` (same shape as Message)."
  def map_message_update(m), do: map_message(m)

  @doc "MESSAGE_DELETE data -> Go `MessageDelete` (only ids survive)."
  def map_message_delete(m) do
    compact(%{
      "id" => id(Map.get(m, :id)) || "",
      "channel_id" => id(Map.get(m, :channel_id)) || "",
      "guild_id" => id(Map.get(m, :guild_id))
    })
    |> Map.put_new("id", "")
    |> Map.put_new("channel_id", "")
  end

  # ── Reactions ──────────────────────────────────────────────────────────────────

  @doc "MESSAGE_REACTION_ADD / MESSAGE_REACTION_REMOVE data -> Go `Reaction`."
  def map_reaction(r) do
    compact(%{
      "user_id" => id(Map.get(r, :user_id)) || "",
      "channel_id" => id(Map.get(r, :channel_id)) || "",
      "message_id" => id(Map.get(r, :message_id)) || "",
      "guild_id" => id(Map.get(r, :guild_id)),
      "emoji" => emoji(Map.get(r, :emoji)),
      "member" => member_or_nil(Map.get(r, :member))
    })
    |> Map.put_new("user_id", "")
    |> Map.put_new("channel_id", "")
    |> Map.put_new("message_id", "")
    |> Map.put_new("emoji", %{"name" => ""})
  end

  defp emoji(nil), do: %{"name" => ""}

  defp emoji(e) do
    compact(%{
      "id" => id(Map.get(e, :id)),
      "name" => Map.get(e, :name) || "",
      "animated" => true_or_nil(Map.get(e, :animated))
    })
    |> Map.put_new("name", "")
  end

  defp member_or_nil(nil), do: nil
  defp member_or_nil(m) when is_map(m), do: member(m, nil)

  # ── Voice ──────────────────────────────────────────────────────────────────────

  @doc "VOICE_STATE_UPDATE data -> Go `VoiceState`. channel_id nil => disconnected."
  def map_voice_state(vs) do
    compact(%{
      "guild_id" => id(Map.get(vs, :guild_id)) || "",
      "channel_id" => id(Map.get(vs, :channel_id)),
      "user_id" => id(Map.get(vs, :user_id)) || "",
      "member" => member_or_nil(Map.get(vs, :member)),
      "session_id" => blank_to_nil(Map.get(vs, :session_id)),
      "deaf" => true_or_nil(Map.get(vs, :deaf)),
      "mute" => true_or_nil(Map.get(vs, :mute)),
      "self_deaf" => true_or_nil(Map.get(vs, :self_deaf)),
      "self_mute" => true_or_nil(Map.get(vs, :self_mute)),
      "self_video" => true_or_nil(Map.get(vs, :self_video)),
      "self_stream" => true_or_nil(Map.get(vs, :self_stream))
    })
    |> Map.put_new("guild_id", "")
    |> Map.put_new("user_id", "")
  end

  # ── Threads ────────────────────────────────────────────────────────────────────

  @doc "THREAD_CREATE / THREAD_DELETE data -> Go `Channel` (a thread is a channel)."
  def map_thread(c), do: channel(c)

  # ── Interaction (the most complex) ──────────────────────────────────────────────

  @doc "INTERACTION_CREATE data -> Go `Interaction`."
  def map_interaction(i) do
    member_struct = Map.get(i, :member)
    user_struct = Map.get(i, :user)

    # interaction.user is the full nested user even for guild interactions
    # (Nostrum copies member.user into it). Build member.user from it.
    member_map =
      case member_struct do
        nil -> nil
        m -> member(m, user(user_struct))
      end

    compact(%{
      "id" => id(Map.get(i, :id)) || "",
      "application_id" => id(Map.get(i, :application_id)) || "",
      "type" => Map.get(i, :type),
      "token" => Map.get(i, :token),
      "version" => Map.get(i, :version),
      "guild_id" => id(Map.get(i, :guild_id)),
      "channel_id" => id(Map.get(i, :channel_id)),
      "member" => member_map,
      "user" => if(member_struct == nil, do: user(user_struct), else: nil),
      "locale" => blank_to_nil(Map.get(i, :locale)),
      "guild_locale" => blank_to_nil(Map.get(i, :guild_locale)),
      "data" => interaction_data(Map.get(i, :data)),
      "message" => message_ref(Map.get(i, :message))
    })
    |> Map.put_new("id", "")
    |> Map.put_new("application_id", "")
    |> Map.put_new("data", %{})
  end

  defp interaction_data(nil), do: %{}

  defp interaction_data(d) do
    compact(%{
      # Application command (type 2) / autocomplete (type 4)
      "id" => id(Map.get(d, :id)),
      "name" => blank_to_nil(Map.get(d, :name)),
      "type" => Map.get(d, :type),
      "options" => list_or_nil(Map.get(d, :options), &interaction_option/1),
      "resolved" => resolved(Map.get(d, :resolved)),
      "target_id" => id(Map.get(d, :target_id)),

      # Message component (type 3)
      "custom_id" => blank_to_nil(Map.get(d, :custom_id)),
      "component_type" => Map.get(d, :component_type),
      "values" => list_or_nil_keep(Map.get(d, :values)),

      # Modal submit (type 5)
      "components" => list_or_nil(Map.get(d, :components), &modal_row/1)
    })
  end

  defp interaction_option(o) do
    compact(%{
      "name" => Map.get(o, :name),
      "type" => Map.get(o, :type) || 0,
      "value" => option_value(Map.get(o, :type), Map.get(o, :value)),
      "options" => list_or_nil(Map.get(o, :options), &interaction_option/1),
      "focused" => true_or_nil(Map.get(o, :focused))
    })
    |> Map.put_new("name", "")
  end

  # For snowflake-typed options (USER 6, CHANNEL 7, ROLE 8, MENTIONABLE 9),
  # Nostrum delivers an integer id; the contract wants a decimal string. Other
  # types (string/int/bool/number) pass through unchanged.
  defp option_value(_type, nil), do: nil
  defp option_value(type, v) when type in [6, 7, 8, 9], do: id(v)
  defp option_value(_type, v), do: v

  defp resolved(nil), do: nil

  defp resolved(r) do
    users = Map.get(r, :users)

    compact(%{
      "users" => map_resolved(users, &user/1),
      "members" => map_resolved_members(Map.get(r, :members), users),
      "roles" => map_resolved(Map.get(r, :roles), &role/1),
      "channels" => map_resolved(Map.get(r, :channels), &channel/1)
    })
  end

  defp map_resolved(nil, _f), do: nil
  defp map_resolved(map, _f) when map == %{}, do: nil

  defp map_resolved(map, f) when is_map(map) do
    Map.new(map, fn {k, v} -> {id(k), f.(v)} end)
  end

  defp map_resolved_members(nil, _users), do: nil
  defp map_resolved_members(map, _users) when map == %{}, do: nil

  defp map_resolved_members(map, users) when is_map(map) do
    Map.new(map, fn {k, m} ->
      user_map =
        case users && Map.get(users, k) do
          nil -> nil
          u -> user(u)
        end

      {id(k), member(m, user_map)}
    end)
  end

  # Modal rows: each `data.components` entry is an action row whose `components`
  # are the submitted text inputs.
  defp modal_row(row) do
    %{
      "type" => Map.get(row, :type) || 1,
      "components" => Enum.map(Map.get(row, :components) || [], &modal_component/1)
    }
  end

  defp modal_component(c) do
    %{
      "type" => Map.get(c, :type) || 0,
      "custom_id" => Map.get(c, :custom_id) || "",
      "value" => Map.get(c, :value) || ""
    }
  end

  defp message_ref(nil), do: nil

  defp message_ref(m) do
    compact(%{
      "id" => id(Map.get(m, :id)) || "",
      "channel_id" => id(Map.get(m, :channel_id))
    })
    |> Map.put_new("id", "")
  end

  # ── Generic list/map helpers ──────────────────────────────────────────────────

  defp list_or_nil(nil, _f), do: nil
  defp list_or_nil([], _f), do: nil
  defp list_or_nil(list, f) when is_list(list), do: Enum.map(list, f)

  defp list_or_nil_ids(nil), do: nil
  defp list_or_nil_ids([]), do: nil
  defp list_or_nil_ids(list) when is_list(list), do: ids(list)

  defp list_or_nil_keep(nil), do: nil
  defp list_or_nil_keep([]), do: nil
  defp list_or_nil_keep(list) when is_list(list), do: list

  # Guild.channels / Guild.roles arrive as a map (%{id => struct}) from Nostrum,
  # but may be a plain list when raw/uncached. Normalize either shape to a list
  # before `list_or_nil/2` (which only accepts lists).
  defp collection_values(nil), do: nil
  defp collection_values(list) when is_list(list), do: list
  defp collection_values(map) when is_map(map), do: Map.values(map)

  defp blank_to_nil(nil), do: nil
  defp blank_to_nil(""), do: nil
  defp blank_to_nil(v), do: v

  defp true_or_nil(true), do: true
  defp true_or_nil(_), do: nil
end
