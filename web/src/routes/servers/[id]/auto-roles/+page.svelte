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
	import { UserPlus, ExternalLink } from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'autorole';

	type Cfg = {
		roles: string[];
		include_bots: boolean;
		wait_for_screening: boolean;
		// The follow-up flow, authored on the automations canvas
		// (/automations/autorole.join). This page never edits it; the onMount
		// spread round-trips it untouched and we only read its length here.
		tail?: unknown[];
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
	const tailSteps = $derived(Array.isArray(cfg.tail) ? cfg.tail.length : 0);

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

<div class="relative flex h-full flex-col bg-bg text-ink">
	<!-- ── Slab topbar ──────────────────────────────────────────────────── -->
	<PageTopbar eyebrow="Auto Roles" subtitle="Give every new member a set of roles automatically.">
		{#snippet leading()}
			<div class="grid size-6 place-items-center rounded border border-line bg-surface text-muted">
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
	<div class="relative min-h-0 flex-1 overflow-y-auto bg-bg pb-20">
		{#if !loaded}
			<div class="p-6">
				<div class="skeleton mb-3 h-6 w-40 rounded"></div>
				<div class="skeleton h-40 w-full rounded"></div>
			</div>
		{:else}
			<div class="grid border-b border-line/60 lg:grid-cols-2 lg:divide-x lg:divide-line/60">
				<!-- ── Left column: Roles · Options ────────────────────── -->
				<div class="min-w-0">
					<!-- ── Roles granted on join ──────────────────────────── -->
					<SectionBar label="Roles granted on join" count={cfg.roles.length || undefined} />
					<div class="px-5 py-5">
						{#if !enabled}
							<div class="mb-4 flex items-center gap-2 border-b border-line/60 pb-4 text-[12px] text-muted">
								<span class="size-1.5 shrink-0 rounded-full bg-faint/40"></span>
								Auto roles is off. Turn it on, top-right, to start assigning roles.
							</div>
						{/if}
						<Field hint="Every new member receives these roles the moment they join.">
							<RolePicker
								multiple
								value={cfg.roles}
								onChange={(v) => (cfg.roles = v as string[])}
								placeholder="Add a role…"
							/>
						</Field>
					</div>

					<!-- ── Options ────────────────────────────────────────── -->
					<SectionBar label="Options" />
					<div class="px-5 py-5">
						<!-- Flat hairline toggle rows (no box). -->
						<div class="flex items-center justify-between gap-4 border-b border-line/60 pb-4">
							<div class="min-w-0">
								<div class="text-[12.5px] font-medium text-ink">Assign to bots</div>
								<div class="mt-0.5 text-[12px] text-muted">Also give these roles to bots when they join.</div>
							</div>
							<label class="flex shrink-0 items-center gap-2 text-[12px]">
								<span class="hidden text-muted sm:inline">{cfg.include_bots ? 'On' : 'Off'}</span>
								<Toggle bind:checked={cfg.include_bots} label="Assign to bots" />
							</label>
						</div>
						<div class="mt-4 flex items-center justify-between gap-4 border-b border-line/60 pb-4">
							<div class="min-w-0">
								<div class="text-[12.5px] font-medium text-ink">Wait for membership screening</div>
								<div class="mt-0.5 text-[12px] text-muted">Hold roles until members pass membership screening before assigning.</div>
							</div>
							<label class="flex shrink-0 items-center gap-2 text-[12px]">
								<span class="hidden text-muted sm:inline">{cfg.wait_for_screening ? 'On' : 'Off'}</span>
								<Toggle bind:checked={cfg.wait_for_screening} label="Wait for screening" />
							</label>
						</div>
					</div>
				</div>

				<!-- ── Right column: What happens on join ──────────────── -->
				<div class="min-w-0">
					<SectionBar label="What happens on join" />
					<div class="px-5 py-5">
						<!-- The live pipeline, as flat numbered rows (no boxes). -->
						<ol>
							<li class="flex items-start gap-3 border-b border-line/60 pb-4">
								<span class="w-6 shrink-0 pt-px font-mono text-[11px] text-faint">01</span>
								<div class="min-w-0">
									<div class="text-[12.5px] font-medium text-ink">Member joins</div>
									<div class="mt-0.5 text-[12px] text-muted">
										{cfg.include_bots ? 'Members and bots trigger the flow.' : 'Members trigger the flow; bots are skipped.'}
									</div>
								</div>
							</li>
							<li class="flex items-start gap-3 border-b border-line/60 py-4 {cfg.wait_for_screening ? '' : 'opacity-50'}">
								<span class="w-6 shrink-0 pt-px font-mono text-[11px] text-faint">02</span>
								<div class="min-w-0">
									<div class="text-[12.5px] font-medium text-ink">Waits for membership screening</div>
									<div class="mt-0.5 text-[12px] text-muted">
										{cfg.wait_for_screening
											? 'Roles are held until the member passes screening.'
											: 'Off. Roles are assigned the moment they join.'}
									</div>
								</div>
							</li>
							<li class="flex items-start gap-3 border-b border-line/60 py-4">
								<span class="w-6 shrink-0 pt-px font-mono text-[11px] text-faint">03</span>
								<div class="min-w-0">
									<div class="text-[12.5px] font-medium text-ink">Roles granted</div>
									<div class="mt-0.5 text-[12px] text-muted">
										{cfg.roles.length
											? `${cfg.roles.length} ${cfg.roles.length === 1 ? 'role' : 'roles'} assigned to the member.`
											: 'No roles selected yet.'}
									</div>
								</div>
							</li>
							<li class="flex items-start gap-3 pt-4">
								<span class="w-6 shrink-0 pt-px font-mono text-[11px] text-faint">04</span>
								<div class="min-w-0">
									<div class="text-[12.5px] font-medium text-ink">Follow-up flow</div>
									<div class="mt-0.5 text-[12px] text-muted">
										{tailSteps
											? `${tailSteps} custom ${tailSteps === 1 ? 'step runs' : 'steps run'} after the roles.`
											: 'No custom steps yet.'}
									</div>
									<p class="mt-2 text-[12px] text-muted">
										Customize what runs after the roles are granted: send a DM, add a delay, grant more roles.
									</p>
									<a
										href={`/servers/${store.id}/automations/autorole.join`}
										class="mt-1.5 inline-flex items-center gap-1 text-[12px] font-medium text-accent-ink hover:underline"
									>
										Edit the flow <ExternalLink size={11} />
									</a>
								</div>
							</li>
						</ol>
					</div>
				</div>
			</div>
		{/if}
	</div>

	<!-- Release dock — the saving experience -->
	{#if loaded}
		<ReleaseDock {dirty} phase={savePhase} error={saveErr} onsave={save} onreset={reset} />
	{/if}
</div>
