<script lang="ts">
	import { onMount } from 'svelte';
	import { getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api, previewImage } from '$lib/api';
	import Field from '$lib/components/Field.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import Select from '$lib/components/Select.svelte';
	import ColorField from '$lib/components/ColorField.svelte';
	import SaveBar from '$lib/components/SaveBar.svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'welcome';

	type Bg = { from?: string; to?: string; angle?: number; color?: string; image_url?: string; blur?: boolean };
	type Card = {
		enabled: boolean;
		preset: string;
		title: string;
		subtitle: string;
		footer: string;
		background: Bg;
		accent_color: string;
		text_color: string;
		sub_text_color: string;
	};
	type Cfg = {
		channel_id: string;
		message: string;
		use_embed: boolean;
		embed_color: string;
		dm_message: string;
		card: Card;
		leave_enabled: boolean;
		leave_channel_id: string;
		leave_message: string;
	};

	function defaults(): Cfg {
		return {
			channel_id: '',
			message: 'Hey {user.mention}, welcome to **{server}**! 🎉',
			use_embed: false,
			embed_color: '#B244FC',
			dm_message: '',
			card: {
				enabled: true,
				preset: 'aurora',
				title: 'Welcome, {user}!',
				subtitle: "You're member #{count} of {server}",
				footer: '',
				background: { from: '#FF6363', to: '#B244FC', angle: 45, color: '', image_url: '', blur: false },
				accent_color: '#FFFFFF',
				text_color: '#FFFFFF',
				sub_text_color: '#F1DFDF'
			},
			leave_enabled: false,
			leave_channel_id: '',
			leave_message: '**{username}** just left. We are now {count} members.'
		};
	}

	let enabled = $state(false);
	let cfg = $state<Cfg>(defaults());
	let loaded = $state(false);
	let saving = $state(false);
	let baseline = $state('');
	let presets = $state<any[]>([]);
	let bgType = $state<'gradient' | 'solid' | 'image'>('gradient');
	let previewUrl = $state('');

	const channelOpts = $derived(store.textChannelOptions());
	const dirty = $derived(loaded && JSON.stringify({ enabled, cfg }) !== baseline);

	onMount(async () => {
		const [f, p] = await Promise.all([api.feature(store.id, FEATURE), api.welcomePresets().catch(() => ({ presets: [] }))]);
		const d = defaults();
		const c = (f.config ?? {}) as Partial<Cfg>;
		cfg = {
			...d,
			...c,
			card: { ...d.card, ...(c.card ?? {}), background: { ...d.card.background, ...((c.card?.background) ?? {}) } }
		};
		enabled = f.enabled;
		presets = p.presets ?? [];
		bgType = cfg.card.background.image_url ? 'image' : cfg.card.background.from && cfg.card.background.to ? 'gradient' : 'solid';
		loaded = true;
		baseline = JSON.stringify({ enabled, cfg });
	});

	function applyPreset(id: string) {
		const p = presets.find((x) => x.id === id);
		if (!p) return;
		cfg.card.preset = id;
		cfg.card.background = { ...cfg.card.background, ...p.background };
		cfg.card.accent_color = p.accent_color;
		cfg.card.text_color = p.text_color;
		cfg.card.sub_text_color = p.sub_text_color;
		bgType = p.background.image_url ? 'image' : p.background.from ? 'gradient' : 'solid';
	}

	function setBgType(t: 'gradient' | 'solid' | 'image') {
		bgType = t;
		if (t === 'gradient') cfg.card.background.image_url = '';
		if (t === 'solid') {
			cfg.card.background.image_url = '';
			cfg.card.background.from = '';
			cfg.card.background.to = '';
			if (!cfg.card.background.color) cfg.card.background.color = '#1F1B2E';
		}
	}

	// Live preview (debounced) — re-renders whenever card fields change.
	let timer: ReturnType<typeof setTimeout>;
	$effect(() => {
		if (!loaded || !cfg.card.enabled) return;
		const payload = {
			background: cfg.card.background,
			accent_color: cfg.card.accent_color,
			text_color: cfg.card.text_color,
			sub_text_color: cfg.card.sub_text_color,
			title: cfg.card.title,
			subtitle: cfg.card.subtitle,
			footer: cfg.card.footer,
			username: 'NewMember',
			count: 1024
		};
		const json = JSON.stringify(payload); // track deps
		clearTimeout(timer);
		timer = setTimeout(async () => {
			try {
				const url = await previewImage(store.id, 'welcome', JSON.parse(json));
				if (previewUrl) URL.revokeObjectURL(previewUrl);
				previewUrl = url;
			} catch {
				/* preview is best-effort */
			}
		}, 400);
	});

	async function save() {
		saving = true;
		try {
			await api.saveFeature(store.id, FEATURE, enabled, cfg);
			if (store.detail) store.detail.features[FEATURE] = { enabled, config: cfg };
			baseline = JSON.stringify({ enabled, cfg });
		} finally {
			saving = false;
		}
	}

	function reset() {
		const b = JSON.parse(baseline);
		enabled = b.enabled;
		cfg = b.cfg;
		bgType = cfg.card.background.image_url ? 'image' : cfg.card.background.from ? 'gradient' : 'solid';
	}
</script>

<svelte:head><title>Welcome · {store.name} · Dia</title></svelte:head>

<header class="mb-6 flex items-start justify-between gap-4">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">Welcome</h1>
		<p class="mt-1 text-muted">Greet new members with a message and a custom welcome card.</p>
	</div>
	<Toggle bind:checked={enabled} />
</header>

{#if !loaded}
	<div class="text-muted">Loading…</div>
{:else}
	<div class="space-y-5">
		<!-- Message -->
		<section class="card p-6">
			<h2 class="mb-4 text-base font-semibold">Message</h2>
			<Field label="Channel" hint="Where welcome messages are posted.">
				<Select bind:value={cfg.channel_id} options={channelOpts} placeholder="Select a channel…" />
			</Field>
			<Field
				label="Message"
				hint="Placeholders: {'{user}'} {'{user.mention}'} {'{username}'} {'{server}'} {'{count}'}"
			>
				<textarea class="input" rows="2" bind:value={cfg.message}></textarea>
			</Field>
			<label class="flex items-center gap-3">
				<Toggle bind:checked={cfg.use_embed} />
				<span class="text-sm">Send as an embed</span>
			</label>
		</section>

		<!-- Welcome card -->
		<section class="card p-6">
			<div class="mb-4 flex items-center justify-between">
				<h2 class="text-base font-semibold">Welcome card</h2>
				<label class="flex items-center gap-2 text-sm text-muted">
					Enabled <Toggle bind:checked={cfg.card.enabled} />
				</label>
			</div>

			{#if cfg.card.enabled}
				<!-- Live preview -->
				<div class="mb-5 overflow-hidden rounded-xl border border-line bg-[#f6f1f4]">
					{#if previewUrl}
						<img src={previewUrl} alt="Welcome card preview" class="w-full" />
					{:else}
						<div class="flex aspect-[1024/450] items-center justify-center text-sm text-faint">
							Rendering preview…
						</div>
					{/if}
				</div>

				<Field label="Preset">
					<div class="flex flex-wrap gap-2">
						{#each presets as p (p.id)}
							<button
								type="button"
								onclick={() => applyPreset(p.id)}
								class="rounded-lg border px-3 py-1.5 text-sm transition-colors {cfg.card.preset === p.id
									? 'border-accent bg-blush text-accent-ink'
									: 'border-line-strong hover:bg-[#faf5f8]'}"
							>
								{p.name}
							</button>
						{/each}
					</div>
				</Field>

				<div class="grid gap-4 sm:grid-cols-2">
					<Field label="Title"><input class="input" bind:value={cfg.card.title} /></Field>
					<Field label="Subtitle"><input class="input" bind:value={cfg.card.subtitle} /></Field>
				</div>

				<Field label="Background">
					<div class="mb-3 inline-flex rounded-lg border border-line-strong p-0.5">
						{#each ['gradient', 'solid', 'image'] as t (t)}
							<button
								type="button"
								onclick={() => setBgType(t as any)}
								class="rounded-md px-3 py-1 text-sm capitalize {bgType === t
									? 'bg-ink text-white'
									: 'text-muted'}"
							>
								{t}
							</button>
						{/each}
					</div>
					{#if bgType === 'gradient'}
						<div class="grid gap-3 sm:grid-cols-2">
							<ColorField label="From" bind:value={cfg.card.background.from} />
							<ColorField label="To" bind:value={cfg.card.background.to} />
						</div>
					{:else if bgType === 'solid'}
						<ColorField label="Color" bind:value={cfg.card.background.color} />
					{:else}
						<input class="input" placeholder="https://image.png" bind:value={cfg.card.background.image_url} />
					{/if}
				</Field>

				<div class="grid gap-4 sm:grid-cols-3">
					<ColorField label="Accent" bind:value={cfg.card.accent_color} />
					<ColorField label="Text" bind:value={cfg.card.text_color} />
					<ColorField label="Subtext" bind:value={cfg.card.sub_text_color} />
				</div>
			{/if}
		</section>

		<!-- Leave -->
		<section class="card p-6">
			<div class="mb-4 flex items-center justify-between">
				<h2 class="text-base font-semibold">Leave message</h2>
				<Toggle bind:checked={cfg.leave_enabled} />
			</div>
			{#if cfg.leave_enabled}
				<Field label="Channel">
					<Select bind:value={cfg.leave_channel_id} options={channelOpts} placeholder="Select a channel…" />
				</Field>
				<Field label="Message">
					<textarea class="input" rows="2" bind:value={cfg.leave_message}></textarea>
				</Field>
			{/if}
		</section>
	</div>

	<SaveBar {dirty} {saving} onsave={save} onreset={reset} />
{/if}
