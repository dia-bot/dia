<script lang="ts">
	import { getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import Toggle from '$lib/components/Toggle.svelte';
	import {
		ImageIcon,
		TrendingUp,
		ToggleRight,
		UserPlus,
		ShieldCheck,
		ShieldAlert,
		Wand2,
		ArrowRight
	} from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);

	const features = [
		{ key: 'welcome', path: 'welcome', label: 'Welcome', icon: ImageIcon, desc: 'Greet new members with messages and images.' },
		{ key: 'leveling', path: 'leveling', label: 'Leveling', icon: TrendingUp, desc: 'XP, levels, rank cards and role rewards.' },
		{ key: 'reactionroles', path: 'reaction-roles', label: 'Reaction Roles', icon: ToggleRight, desc: 'Self-assignable roles via buttons & menus.' },
		{ key: 'autorole', path: 'auto-roles', label: 'Auto Roles', icon: UserPlus, desc: 'Automatically assign roles on join.' },
		{ key: 'moderation', path: 'moderation', label: 'Moderation', icon: ShieldCheck, desc: 'Ban, kick, timeout, warn — with a case log.' },
		{ key: 'automod', path: 'automod', label: 'Automod', icon: ShieldAlert, desc: 'Auto-filter spam, links and banned words.' },
		{ key: 'customcommands', path: 'commands', label: 'Custom Commands', icon: Wand2, desc: 'Build your own slash commands.' }
	];

	let busy = $state<string | null>(null);

	async function toggle(key: string) {
		const f = store.feature(key);
		busy = key;
		try {
			await api.saveFeature(store.id, key, !f.enabled, f.config);
			if (store.detail) {
				store.detail.features[key] = { enabled: !f.enabled, config: f.config };
			}
		} finally {
			busy = null;
		}
	}
</script>

<svelte:head><title>{store.name} · Dia</title></svelte:head>

<div class="mb-6">
	<h1 class="text-2xl font-bold tracking-tight">Dashboard</h1>
	<p class="mt-1 text-muted">Enable features and click through to configure each one.</p>
</div>

<div class="grid gap-3 sm:grid-cols-2">
	{#each features as f (f.key)}
		{@const state = store.feature(f.key)}
		<div class="card flex flex-col p-5">
			<div class="flex items-start justify-between">
				<div class="grid h-10 w-10 place-items-center rounded-xl bg-blush text-accent">
					<f.icon size={20} />
				</div>
				<Toggle checked={state.enabled} disabled={busy === f.key} onchange={() => toggle(f.key)} />
			</div>
			<h3 class="mt-4 font-semibold">{f.label}</h3>
			<p class="mt-1 flex-1 text-sm text-muted">{f.desc}</p>
			<a
				href="/servers/{store.id}/{f.path}"
				class="mt-4 inline-flex items-center gap-1 text-sm font-medium text-accent-ink hover:underline"
			>
				Configure <ArrowRight size={14} />
			</a>
		</div>
	{/each}
</div>
