// Provider presentation for the social alerts UI: a lucide icon + brand accent
// per platform. The catalogue itself (which providers exist, which are
// unlocked) comes from the API; this is only how we draw each one. Shared by the
// Social page and the onboarding wizard so they never drift.

import { Megaphone, Twitch, Youtube, Radio, Cloud, Rss, Twitter, Instagram, Music2 } from 'lucide-svelte';

export const PROVIDER_ICONS: Record<string, typeof Megaphone> = {
	twitch: Twitch,
	youtube: Youtube,
	kick: Radio,
	bluesky: Cloud,
	rss: Rss,
	x: Twitter,
	instagram: Instagram,
	tiktok: Music2
};

export const PROVIDER_COLORS: Record<string, string> = {
	twitch: '#9146FF',
	youtube: '#FF0000',
	kick: '#53FC18',
	bluesky: '#0085FF',
	rss: '#ff6363'
};

export function providerIcon(provider: string): typeof Megaphone {
	return PROVIDER_ICONS[provider] ?? Megaphone;
}

export function providerColor(provider: string): string {
	return PROVIDER_COLORS[provider] ?? 'var(--color-muted)';
}
