// TanStack Query (svelte-query) hooks for the dashboard. A single ['guilds']
// query backs both the server switcher and the command palette, so opening
// either is instant and they share one cache + background refresh.
import { createQuery } from '@tanstack/svelte-query';
import { api } from './api';
import type { GuildListItem } from './types';

// presentGuildsQuery returns the servers the user manages where Dia is present.
export function presentGuildsQuery() {
	return createQuery(() => ({
		queryKey: ['guilds'],
		queryFn: async () => (await api.guilds()).guilds,
		staleTime: 30_000,
		select: (guilds: GuildListItem[]) => guilds.filter((g) => g.bot_present)
	}));
}
