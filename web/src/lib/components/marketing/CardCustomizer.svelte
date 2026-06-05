<script lang="ts">
	import { onMount } from 'svelte';
	import WelcomeCard from './WelcomeCard.svelte';
	import ColorField from '$lib/components/ColorField.svelte';

	// The actual dashboard welcome-card editor, embedded live on the marketing
	// page. The four presets are the real Dia-branded built-ins. Until a visitor
	// touches a control it gently auto-cycles presets so the card visibly "moves".
	type Preset = {
		id: string;
		name: string;
		from: string;
		to: string;
		angle: number;
		color: string;
		accent: string;
		text: string;
		subtext: string;
	};

	const PRESETS: Preset[] = [
		{ id: 'aurora', name: 'Aurora', from: '#FF6363', to: '#B244FC', angle: 45, color: '', accent: '#FFFFFF', text: '#FFFFFF', subtext: '#F7E9F2' },
		{ id: 'midnight', name: 'Midnight', from: '#1F1B2E', to: '#3A2E5C', angle: 30, color: '', accent: '#B244FC', text: '#FFFFFF', subtext: '#C9C3DA' },
		{ id: 'blush', name: 'Blush', from: '', to: '', angle: 0, color: '#F1DFDF', accent: '#FF6363', text: '#2B2233', subtext: '#7A6B73' },
		{ id: 'sunset', name: 'Sunset', from: '#FF6363', to: '#FFB347', angle: 60, color: '', accent: '#FFFFFF', text: '#FFFFFF', subtext: '#FFF1E6' }
	];

	let preset = $state('aurora');
	let from = $state(PRESETS[0].from);
	let to = $state(PRESETS[0].to);
	let angle = $state(PRESETS[0].angle);
	let color = $state(PRESETS[0].color);
	let accent = $state(PRESETS[0].accent);
	let text = $state(PRESETS[0].text);
	let subtext = $state(PRESETS[0].subtext);
	let bgType = $state<'gradient' | 'solid'>('gradient');
	let title = $state('Welcome, {user}!');
	let subtitle = $state("You're member #{count} of {server}");

	let touched = $state(false);

	function applyPreset(p: Preset, fromUser = true) {
		preset = p.id;
		from = p.from;
		to = p.to;
		angle = p.angle;
		color = p.color;
		accent = p.accent;
		text = p.text;
		subtext = p.subtext;
		bgType = p.from && p.to ? 'gradient' : 'solid';
		if (fromUser) touch();
	}

	function touch() {
		touched = true;
	}

	function setBgType(t: 'gradient' | 'solid') {
		bgType = t;
		if (t === 'solid' && !color) color = '#1F1B2E';
		if (t === 'gradient' && (!from || !to)) {
			from = from || '#FF6363';
			to = to || '#B244FC';
		}
		touch();
	}

	onMount(() => {
		const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
		if (reduce) return;
		let i = 0;
		const id = setInterval(() => {
			if (touched) {
				clearInterval(id);
				return;
			}
			i = (i + 1) % PRESETS.length;
			applyPreset(PRESETS[i], false);
		}, 3200);
		return () => clearInterval(id);
	});
</script>

<div class="card overflow-hidden p-4 sm:p-5" role="group" onpointerdown={touch} onfocusin={touch}>
	<div class="mb-4 flex items-center justify-between">
		<div class="flex items-center gap-2">
			<h3 class="text-[15px] font-semibold">Welcome card</h3>
			<span
				class="inline-flex items-center gap-1.5 rounded-full bg-blush px-2 py-0.5 text-[11px] font-semibold text-accent-ink"
			>
				<span class="h-1.5 w-1.5 rounded-full bg-accent"></span> Live preview
			</span>
		</div>
		<span class="hidden text-xs text-muted sm:block">Try the controls →</span>
	</div>

	<!-- live preview -->
	<div class="rounded-2xl bg-ink-2 p-2.5 ring-1 ring-line">
		<WelcomeCard {from} {to} {angle} {color} {accent} {text} {subtext} {title} {subtitle} />
	</div>

	<!-- presets -->
	<div class="mt-5">
		<span class="label">Preset</span>
		<div class="flex flex-wrap gap-2">
			{#each PRESETS as p (p.id)}
				<button
					type="button"
					onclick={() => applyPreset(p)}
					aria-pressed={preset === p.id}
					class="rounded-lg border px-3 py-1.5 text-sm transition-colors {preset === p.id
						? 'border-accent bg-blush text-accent-ink'
						: 'border-line-strong hover:bg-ink-2'}"
				>
					{p.name}
				</button>
			{/each}
		</div>
	</div>

	<!-- text -->
	<div class="mt-4 grid gap-4 sm:grid-cols-2">
		<div>
			<span class="label">Title</span>
			<input class="input" bind:value={title} oninput={touch} />
		</div>
		<div>
			<span class="label">Subtitle</span>
			<input class="input" bind:value={subtitle} oninput={touch} />
		</div>
	</div>

	<!-- background -->
	<div class="mt-4">
		<span class="label">Background</span>
		<div class="mb-3 inline-flex rounded-lg border border-line-strong p-0.5">
			{#each ['gradient', 'solid'] as t (t)}
				<button
					type="button"
					onclick={() => setBgType(t as 'gradient' | 'solid')}
					aria-pressed={bgType === t}
					class="rounded-md px-3 py-1 text-sm capitalize {bgType === t
						? 'bg-ink text-bg'
						: 'text-muted'}"
				>
					{t}
				</button>
			{/each}
		</div>
		{#if bgType === 'gradient'}
			<div class="grid gap-3 sm:grid-cols-2">
				<ColorField label="From" bind:value={from} />
				<ColorField label="To" bind:value={to} />
			</div>
		{:else}
			<ColorField label="Color" bind:value={color} />
		{/if}
	</div>

	<!-- palette -->
	<div class="mt-4 grid gap-3 sm:grid-cols-3">
		<ColorField label="Accent" bind:value={accent} />
		<ColorField label="Text" bind:value={text} />
		<ColorField label="Subtext" bind:value={subtext} />
	</div>
</div>
