<script lang="ts">
	// Billing & Storage: plan status (Stripe checkout/manage) + the server's
	// uploaded-asset overview with usage vs quota and per-asset delete.
	import { getContext, onMount } from 'svelte';
	import { page } from '$app/stores';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import {
		billingStatus,
		billingCheckout,
		billingPortal,
		guildAssets,
		deleteAsset,
		type BillingStatus,
		type AssetItem
	} from '$lib/api';
	import { Check, Crown, CreditCard, Trash2, Loader2, Image as ImageIcon, Type } from 'lucide-svelte';

	const guild = getContext<GuildStore>(GUILD_CTX);

	let bill = $state<BillingStatus | null>(null);
	let assets = $state<AssetItem[]>([]);
	let used = $state(0);
	let quota = $state(0);
	let premium = $state(false);
	let loading = $state(true);
	let acting = $state(false);
	let err = $state('');

	const pct = $derived(quota > 0 ? Math.min(100, Math.round((used / quota) * 100)) : 0);
	const checkoutResult = $derived($page.url.searchParams.get('checkout'));

	function human(n: number): string {
		if (n < 1024) return `${n} B`;
		const u = ['KB', 'MB', 'GB', 'TB'];
		let v = n / 1024;
		let i = 0;
		while (v >= 1024 && i < u.length - 1) {
			v /= 1024;
			i++;
		}
		return `${v.toFixed(1)} ${u[i]}`;
	}

	async function load() {
		try {
			const [b, a] = await Promise.all([billingStatus(guild.id), guildAssets(guild.id)]);
			bill = b;
			assets = a.assets;
			used = a.used;
			quota = a.quota;
			premium = a.premium;
			err = '';
		} catch (e) {
			err = e instanceof Error ? e.message : 'Could not load billing & storage';
		} finally {
			loading = false;
		}
	}
	const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));
	onMount(async () => {
		await load();
		// After returning from checkout the webhook may not have landed yet —
		// poll briefly until premium flips on.
		if (checkoutResult === 'success') {
			for (let i = 0; i < 6 && !premium; i++) {
				await sleep(2500);
				await load();
			}
		}
	});

	async function upgrade() {
		acting = true;
		try {
			const { url } = await billingCheckout(guild.id);
			window.location.href = url;
		} catch (e) {
			acting = false;
			alert(e instanceof Error ? e.message : 'Could not start checkout');
		}
	}
	async function manage() {
		acting = true;
		try {
			const { url } = await billingPortal(guild.id);
			window.location.href = url;
		} catch (e) {
			acting = false;
			alert(e instanceof Error ? e.message : 'Could not open billing portal');
		}
	}
	async function remove(a: AssetItem) {
		try {
			await deleteAsset(guild.id, a.id);
			assets = assets.filter((x) => x.id !== a.id);
			used = Math.max(0, used - a.bytes);
			err = '';
		} catch (e) {
			err = e instanceof Error ? e.message : 'Could not delete asset';
		}
	}
</script>

<svelte:head><title>Billing & Storage · {guild.name} · Dia</title></svelte:head>

<header class="mb-6">
	<h1 class="text-xl font-semibold text-ink">Billing & Storage</h1>
	<p class="mt-1 text-sm text-muted">Manage your plan and the assets uploaded to this server.</p>
</header>

{#if checkoutResult === 'success'}
	<div class="mb-4 flex items-center gap-2 rounded-lg border border-success/30 bg-success/10 px-4 py-2.5 text-sm text-success">
		<Check size={15} /> Subscription active — thanks for going Premium!
	</div>
{:else if checkoutResult === 'cancel'}
	<div class="mb-4 rounded-lg border border-line bg-surface px-4 py-2.5 text-sm text-muted">
		Checkout cancelled — no charge was made.
	</div>
{/if}

{#if err}
	<div class="mb-4 rounded-lg border border-danger/30 bg-danger/10 px-4 py-2.5 text-sm text-danger">{err}</div>
{/if}

{#if loading}
	<div class="space-y-3">
		<div class="skeleton h-28 w-full rounded-card"></div>
		<div class="skeleton h-40 w-full rounded-card"></div>
	</div>
{:else}
	<!-- Plan -->
	<section class="card mb-5 overflow-hidden bg-surface">
		<div class="flex flex-wrap items-center justify-between gap-4 p-5">
			<div class="flex items-start gap-3">
				<div class="grid h-10 w-10 place-items-center rounded-xl {premium ? 'bg-accent/15 text-accent-ink' : 'bg-ink-2 text-muted'}">
					<Crown size={18} />
				</div>
				<div>
					<div class="flex items-center gap-2">
						<h2 class="text-sm font-semibold text-ink">{premium ? 'Premium' : 'Free'} plan</h2>
						{#if bill?.status}
							<span class="rounded-full border border-line-strong px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-wide text-faint">{bill.status}</span>
						{/if}
					</div>
					<p class="mt-0.5 text-xs text-muted">
						{#if premium}
							Custom fonts, 5 GB storage{#if bill?.current_period_end}, renews {new Date(bill.current_period_end * 1000).toLocaleDateString()}{/if}.
						{:else}
							500 MB storage. Upgrade for custom fonts and 5 GB — {bill?.price ?? '$3.99/mo'}.
						{/if}
					</p>
				</div>
			</div>
			<div class="flex items-center gap-2">
				{#if premium && !bill?.manage}
					<span class="flex items-center gap-1.5 text-xs font-medium text-accent-ink"><Check size={14} /> Premium active</span>
				{:else if !bill?.billing_enabled}
					<span class="text-xs text-faint">Billing isn't configured on this instance.</span>
				{:else if bill?.manage}
					<button type="button" onclick={manage} disabled={acting} class="flex h-9 items-center gap-1.5 rounded-lg border border-line-strong px-3 text-[13px] font-medium text-ink transition-colors hover:bg-ink-2 disabled:opacity-50">
						{#if acting}<Loader2 size={14} class="animate-spin" />{:else}<CreditCard size={14} />{/if} Manage plan
					</button>
				{:else}
					<button type="button" onclick={upgrade} disabled={acting} class="flex h-9 items-center gap-1.5 rounded-lg bg-accent px-3.5 text-[13px] font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50">
						{#if acting}<Loader2 size={14} class="animate-spin" />{:else}<Crown size={14} />{/if} Upgrade — {bill?.price ?? '$3.99/mo'}
					</button>
				{/if}
			</div>
		</div>
	</section>

	<!-- Storage -->
	<section class="card overflow-hidden bg-surface">
		<div class="border-b border-line p-5">
			<div class="mb-2 flex items-baseline justify-between">
				<h2 class="text-sm font-semibold text-ink">Storage</h2>
				<span class="font-mono text-xs tabular-nums text-muted">{human(used)} / {human(quota)}</span>
			</div>
			<div class="h-2 w-full overflow-hidden rounded-full bg-ink-2">
				<div class="h-full rounded-full transition-[width] {pct >= 90 ? 'bg-danger' : 'bg-accent'}" style="width:{pct}%"></div>
			</div>
			{#if !premium}
				<p class="mt-2 text-[11px] text-faint">
					On the free plan, downgrading keeps your existing files — you just can't upload new ones once you're over 500 MB.
				</p>
			{/if}
		</div>

		{#if assets.length === 0}
			<p class="p-6 text-center text-sm text-faint">No uploads yet. Add images or fonts from the Card Studio.</p>
		{:else}
			<ul class="divide-y divide-line">
				{#each assets as a (a.id)}
					<li class="flex items-center gap-3 px-5 py-2.5">
						{#if a.kind === 'image'}
							<img src={a.url} alt="" class="h-9 w-9 shrink-0 rounded-md border border-line object-cover" />
						{:else}
							<div class="grid h-9 w-9 shrink-0 place-items-center rounded-md border border-line bg-ink-2 text-faint">
								<Type size={15} />
							</div>
						{/if}
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-1.5 text-[13px] text-ink">
								{#if a.kind === 'image'}<ImageIcon size={12} class="text-faint" />{/if}
								<span class="truncate">{a.kind === 'font' ? a.family : a.url.split('/').pop()}</span>
							</div>
							<div class="font-mono text-[11px] text-faint">{a.kind} · {human(a.bytes)}</div>
						</div>
						<button type="button" onclick={() => remove(a)} aria-label="Delete asset" class="grid h-8 w-8 shrink-0 place-items-center rounded-md text-faint transition-colors hover:bg-ink-2 hover:text-danger">
							<Trash2 size={14} />
						</button>
					</li>
				{/each}
			</ul>
		{/if}
	</section>
{/if}
