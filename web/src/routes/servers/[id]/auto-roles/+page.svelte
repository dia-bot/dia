<script lang="ts">
	import { onMount, getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import Toggle from '$lib/components/Toggle.svelte';
	import Field from '$lib/components/Field.svelte';
	import RolePicker from '$lib/components/RolePicker.svelte';
	import PageTopbar from '$lib/components/page/PageTopbar.svelte';
	import SectionBar from '$lib/components/page/SectionBar.svelte';
	import ReleaseDock from '$lib/components/page/ReleaseDock.svelte';
	import { UserPlus } from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'autorole';

	type Cfg = {
		roles: string[];
		include_bots: boolean;
		wait_for_screening: boolean;
	};

	function defaults(): Cfg {
		return {
			roles: [],
			include_bots: false,
			wait_for_screening: false
		};
	}

	let enabled = $state(false);
	let cfg = $state<Cfg>(defaults());
	let loaded = $state(false);
	let baseline = $state('');

	let savePhase = $state<'idle' | 'saving' | 'saved' | 'error'>('idle');
	let saveErr = $state('');

	function serialize() {
		return JSON.stringify({ enabled, cfg });
	}
	const dirty = $derived(loaded && serialize() !== baseline);

	onMount(async () => {
		const f = await api.feature(store.id, FEATURE);
		const c = (f.config ?? {}) as Partial<Cfg>;
		cfg = { ...defaults(), ...c };
		enabled = f.enabled;
		loaded = true;
		baseline = serialize();
	});

	async function save() {
		if (savePhase === 'saving' || !dirty) return;
		savePhase = 'saving';
		saveErr = '';
		try {
			await api.saveFeature(store.id, FEATURE, enabled, cfg);
			if (store.detail)
				store.detail.features[FEATURE] = { enabled, config: cfg as unknown as Record<string, unknown> };
			baseline = serialize();
			savePhase = 'saved';
			setTimeout(() => {
				if (savePhase === 'saved') savePhase = 'idle';
			}, 1800);
		} catch (e) {
			saveErr = e instanceof Error ? e.message : 'Something went wrong';
			savePhase = 'error';
		}
	}

	function reset() {
		const b = JSON.parse(baseline);
		enabled = b.enabled;
		cfg = b.cfg;
		savePhase = 'idle';
		saveErr = '';
	}

	function onKeydown(e: KeyboardEvent) {
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
			e.preventDefault();
			if (dirty) save();
		}
	}
</script>

<svelte:head><title>Auto Roles · {store.name} · Dia</title></svelte:head>
<svelte:window onkeydown={onKeydown} />

<div class="flex h-full flex-col bg-bg text-ink">
	<!-- ── Slab topbar ──────────────────────────────────────────────────── -->
	<PageTopbar eyebrow="Auto Roles" subtitle="Give every new member a set of roles automatically.">
		{#snippet leading()}
			<div class="grid size-6 place-items-center rounded border border-line bg-surface text-accent-ink">
				<UserPlus size={13} />
			</div>
		{/snippet}
		{#snippet actions()}
			<label class="ml-1 flex items-center gap-2 text-[12px]">
				<span class="hidden text-muted sm:inline">{enabled ? 'On' : 'Off'}</span>
				<Toggle bind:checked={enabled} label="Auto roles" />
			</label>
		{/snippet}
	</PageTopbar>

	<!-- ── Body ─────────────────────────────────────────────────────────── -->
	<div class="relative min-h-0 flex-1 overflow-y-auto bg-bg">
		{#if !loaded}
			<div class="mx-auto w-full max-w-2xl p-6">
				<div class="skeleton mb-3 h-6 w-40 rounded"></div>
				<div class="skeleton h-40 w-full rounded-xl"></div>
			</div>
		{:else}
			<div class="mx-auto w-full max-w-2xl space-y-6 px-5 py-6">
				{#if !enabled}
					<div class="flex items-center gap-2 rounded-lg border border-line bg-ink-2 px-3 py-2 text-[12px] text-muted">
						<span class="size-1.5 shrink-0 rounded-full bg-faint/40"></span>
						Auto roles is off. Turn it on, top-right, to start assigning roles.
					</div>
				{/if}

				<!-- Roles granted on join -->
				<section class="space-y-3">
					<SectionBar label="Roles granted on join" class="-mx-5 border-t border-line/60" count={cfg.roles.length || undefined} />
					<Field hint="Every new member receives these roles the moment they join.">
						<RolePicker
							multiple
							value={cfg.roles}
							onChange={(v) => (cfg.roles = v as string[])}
							placeholder="Add a role…"
						/>
					</Field>
				</section>

				<!-- Options -->
				<section class="space-y-3">
					<SectionBar label="Options" class="-mx-5 border-t border-line/60" />
					<label class="flex items-center justify-between gap-3 rounded-xl border border-line bg-surface px-3.5 py-3">
						<span class="min-w-0 text-[12.5px] text-ink">Also assign these roles to bots</span>
						<Toggle bind:checked={cfg.include_bots} label="Assign to bots" />
					</label>
					<label class="flex items-center justify-between gap-3 rounded-xl border border-line bg-surface px-3.5 py-3">
						<span class="min-w-0 text-[12.5px] text-ink">Wait until members pass membership screening before assigning</span>
						<Toggle bind:checked={cfg.wait_for_screening} label="Wait for screening" />
					</label>
				</section>
			</div>
		{/if}

		<!-- Release dock — the saving experience -->
		{#if loaded}
			<ReleaseDock {dirty} phase={savePhase} error={saveErr} onsave={save} onreset={reset} />
		{/if}
	</div>
</div>
