// TanStack Query (svelte-query) hooks for the dashboard. A single ['guilds']
// query backs both the server switcher and the command palette, so opening
// either is instant and they share one cache + background refresh.
import { createQuery } from '@tanstack/svelte-query';
import { api } from './api';
import type { GuildListItem } from './types';

// manageableGuildsQuery returns every server the user manages (Administrator
// or owner), regardless of whether Dia is currently present in it. The
// `bot_present` flag is preserved on each item so callers can badge the ones
// that still need an invite. The switcher and command palette want navigation,
// not presence filtering — hiding non-bot guilds caused the "no servers found"
// empty state when the bot's live guild list was momentarily unreachable.
export function manageableGuildsQuery() {
	return createQuery(() => ({
		queryKey: ['guilds'],
		queryFn: async () => (await api.guilds()).guilds,
		staleTime: 30_000,
		select: (guilds: GuildListItem[]) => guilds
	}));
}
