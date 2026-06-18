<script lang="ts">
	import { onMount, getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import { defaultVerification, type VerificationConfig, type VerifyMode } from '$lib/verification/types';
	import Toggle from '$lib/components/Toggle.svelte';
	import Select from '$lib/components/Select.svelte';
	import RolePicker from '$lib/components/RolePicker.svelte';
	import ChannelSelect from '$lib/components/ChannelSelect.svelte';
	import NumberField from '$lib/components/ui/NumberField.svelte';
	import SaveBar from '$lib/components/SaveBar.svelte';
	import { UserCheck, MousePointerClick, ShieldQuestion, Lock, Clock, ArrowRight } from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'verification';

	let enabled = $state(false);
	let cfg = $state<VerificationConfig>(defaultVerification());
	let loaded = $state(false);
	let saving = $state(false);
	let baseline = $state('');

	const dirty = $derived(loaded && JSON.stringify({ enabled, cfg }) !== baseline);

	const modeOptions = [
		{ value: 'button', label: 'Button click' },
		{ value: 'captcha', label: 'Captcha challenge' }
	];

	onMount(async () => {
		const f = await api.feature(store.id, FEATURE);
		const d = defaultVerification();
		const c = (f.config ?? {}) as Partial<VerificationConfig>;
		cfg = { ...d, ...c };
		enabled = f.enabled;
		loaded = true;
		baseline = JSON.stringify({ enabled, cfg });
	});

	async function save() {
		saving = true;
		try {
			await api.saveFeature(store.id, FEATURE, enabled, cfg);
			if (store.detail)
				store.detail.features[FEATURE] = {
					enabled,
					config: cfg as unknown as Record<string, unknown>
				};
			baseline = JSON.stringify({ enabled, cfg });
		} finally {
			saving = false;
		}
	}
	function reset() {
		const b = JSON.parse(baseline);
		enabled = b.enabled;
		cfg = b.cfg;
	}

	function setMode(v: string) {
		cfg.mode = v as VerifyMode;
	}
</script>

<svelte:head><title>Verification · {store.name} · Dia</title></svelte:head>

<header class="mb-6 flex items-start justify-between gap-4">
	<div class="flex items-start gap-3">
		<span class="mt-0.5 grid size-9 shrink-0 place-items-center rounded-xl border border-line bg-ink-2 text-accent-ink">
			<UserCheck size={18} />
		</span>
		<div>
			<h1 class="text-xl font-semibold tracking-tight text-ink">Verification</h1>
			<p class="mt-1 max-w-xl text-sm text-muted">
				Gate new members behind a quick human check before they can see the rest of the server. Stops
				drive-by raids and bot accounts at the door.
			</p>
		</div>
	</div>
	<label class="flex shrink-0 items-center gap-2.5 text-sm">
		<span class="text-muted">{enabled ? 'Enabled' : 'Disabled'}</span>
		<Toggle bind:checked={enabled} />
	</label>
</header>

{#if !loaded}
	<div class="space-y-4">
		<div class="skeleton h-32 w-full rounded-card"></div>
		<div class="skeleton h-64 w-full rounded-card"></div>
	</div>
{:else}
	<div class="space-y-5">
		<!-- How it works -->
		<section class="rounded-card border border-line bg-ink-2 p-5">
			<div class="eyebrow mb-3">How it works</div>
			<ol class="grid gap-3 text-xs text-muted sm:grid-cols-3">
				<li class="flex items-start gap-2.5">
					<span class="grid size-6 shrink-0 place-items-center rounded-full border border-line-strong bg-surface font-mono text-[11px] font-semibold text-accent-ink">1</span>
					<span>A member joins and is given the <strong class="text-ink">unverified role</strong>, which locks them out of your channels.</span>
				</li>
				<li class="flex items-start gap-2.5">
					<span class="grid size-6 shrink-0 place-items-center rounded-full border border-line-strong bg-surface font-mono text-[11px] font-semibold text-accent-ink">2</span>
					<span>They complete the check posted in the <strong class="text-ink">gate channel</strong> (a button tap or captcha).</span>
				</li>
				<li class="flex items-start gap-2.5">
					<span class="grid size-6 shrink-0 place-items-center rounded-full border border-line-strong bg-surface font-mono text-[11px] font-semibold text-accent-ink">3</span>
					<span>The unverified role is removed (and an optional <strong class="text-ink">verified role</strong> added), unlocking the server.</span>
				</li>
			</ol>
		</section>

		<!-- Mode -->
		<section class="card p-5">
			<div class="eyebrow mb-4">Challenge</div>
			<div class="max-w-xs">
				<span class="label">Verification mode</span>
				<Select bind:value={() => cfg.mode, setMode} options={modeOptions} />
			</div>
			<div class="mt-4 grid gap-3 sm:grid-cols-2">
				<div class="flex items-start gap-2.5 rounded-xl border px-3.5 py-3 text-xs {cfg.mode === 'button' ? 'border-line-strong bg-ink-2' : 'border-line'}">
					<MousePointerClick size={16} class="mt-0.5 shrink-0 text-accent-ink" />
					<div>
						<div class="text-sm font-medium text-ink">Button click</div>
						<p class="mt-0.5 text-muted">Lowest friction. A single button verifies instantly. Best for trusted communities.</p>
					</div>
				</div>
				<div class="flex items-start gap-2.5 rounded-xl border px-3.5 py-3 text-xs {cfg.mode === 'captcha' ? 'border-line-strong bg-ink-2' : 'border-line'}">
					<ShieldQuestion size={16} class="mt-0.5 shrink-0 text-accent-ink" />
					<div>
						<div class="text-sm font-medium text-ink">Captcha challenge</div>
						<p class="mt-0.5 text-muted">Adds a short human-only puzzle. Stronger against automated bot accounts.</p>
					</div>
				</div>
			</div>
		</section>

		<!-- Roles & channel -->
		<section class="card p-5">
			<div class="eyebrow mb-4">Roles &amp; gate</div>
			<div class="grid gap-5 md:grid-cols-2">
				<div>
					<span class="label">Unverified role</span>
					<RolePicker
						value={cfg.unverified_role}
						onChange={(v) => (cfg.unverified_role = v as string)}
						placeholder="Select a role…"
					/>
					<p class="hint">
						<Lock size={11} class="-mt-0.5 mr-0.5 inline text-faint" />
						Assigned the moment a member joins. You must set this role's permissions so it
						<strong class="text-ink">cannot view your channels</strong> (deny View Channel at the
						category level), otherwise the gate does nothing.
					</p>
				</div>
				<div>
					<span class="label">Verified role <span class="text-faint">(optional)</span></span>
					<RolePicker
						value={cfg.verified_role}
						onChange={(v) => (cfg.verified_role = v as string)}
						placeholder="Select a role…"
					/>
					<p class="hint">Granted on success. Handy if you prefer to open channels to a verified role rather than @everyone.</p>
				</div>
				<div class="md:col-span-2 md:max-w-sm">
					<span class="label">Gate channel</span>
					<ChannelSelect bind:value={cfg.channel} />
					<p class="hint">Where the verification prompt is posted. Usually a read-only channel the unverified role can see.</p>
				</div>
			</div>
		</section>

		<!-- Welcome text -->
		<section class="card p-5">
			<div class="eyebrow mb-4">Prompt message</div>
			<textarea
				class="input min-h-[5rem]"
				rows="3"
				placeholder="Welcome! Click below to verify…"
				bind:value={cfg.welcome_text}
			></textarea>
			<p class="hint">
				Shown above the verify control. Supports template values like
				<span class="font-mono text-[11px]">{'{{ .User.Mention }}'}</span> and
				<span class="font-mono text-[11px]">{'{{ .Guild.Name }}'}</span>.
			</p>
		</section>

		<!-- Targeting & timeout -->
		<section class="card p-5">
			<div class="eyebrow mb-4">Targeting &amp; cleanup</div>
			<div class="space-y-5">
				<div class="flex items-start justify-between gap-4">
					<div>
						<div class="text-sm font-medium text-ink">Only challenge suspicious accounts</div>
						<p class="text-xs text-muted">Skip the gate for established accounts and only verify brand-new ones.</p>
					</div>
					<Toggle bind:checked={cfg.only_suspicious} label="Only suspicious" />
				</div>
				{#if cfg.only_suspicious}
					<div class="max-w-xs">
						<span class="label">Minimum account age</span>
						<div class="flex items-center gap-2">
							<div class="w-28">
								<NumberField bind:value={cfg.min_account_age_hours} min={1} max={8760} />
							</div>
							<span class="text-xs text-muted">hours, below which an account is gated</span>
						</div>
					</div>
				{/if}
				<div class="max-w-md">
					<span class="label">Kick if not verified within</span>
					<div class="flex items-center gap-2">
						<div class="w-28">
							<NumberField bind:value={cfg.kick_after_minutes} min={0} max={1440} />
						</div>
						<span class="text-xs text-muted">
							<Clock size={11} class="-mt-0.5 mr-0.5 inline text-faint" />
							minutes ({cfg.kick_after_minutes === 0 ? 'never kick' : `auto-kick after ${cfg.kick_after_minutes}m`})
						</span>
					</div>
					<p class="hint">Set to 0 to leave unverified members in place indefinitely.</p>
				</div>
			</div>
		</section>

		<a
			href="/servers/{store.id}/logging"
			class="inline-flex items-center gap-1.5 text-xs font-medium text-accent-ink hover:underline"
		>
			Log every join and verification to a channel <ArrowRight size={13} />
		</a>
	</div>

	<SaveBar {dirty} {saving} onsave={save} onreset={reset} />
{/if}
