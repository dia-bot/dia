<script lang="ts">
	import { untrack } from 'svelte';
	import { THEMES, templateElements, nextId } from './types';
	import type { El, ElKind, Background } from './types';
	import WelcomeCanvas from './WelcomeCanvas.svelte';
	import ColorField from '$lib/components/ColorField.svelte';
	import Plus from 'lucide-svelte/icons/plus';
	import Trash2 from 'lucide-svelte/icons/trash-2';
	import Eye from 'lucide-svelte/icons/eye';
	import EyeOff from 'lucide-svelte/icons/eye-off';
	import ChevronUp from 'lucide-svelte/icons/chevron-up';
	import ChevronDown from 'lucide-svelte/icons/chevron-down';
	import RotateCcw from 'lucide-svelte/icons/rotate-ccw';

	// The element-based card editor — layout templates, theme presets, a layer
	// list, draggable canvas, and a per-element inspector. Works for welcome cards
	// and (mode="rank") rank cards. A real composer, not a fixed form.
	let { mode = 'welcome' }: { mode?: 'welcome' | 'rank' } = $props();

	const WELCOME_TEMPLATES = [
		{ id: 'centered', name: 'Centered' },
		{ id: 'banner', name: 'Banner' },
		{ id: 'split', name: 'Split' },
		{ id: 'spotlight', name: 'Spotlight' },
		{ id: 'minimal', name: 'Minimal' },
		{ id: 'stacked', name: 'Stacked' }
	];
	const RANK_TEMPLATES = [
		{ id: 'classic', name: 'Classic' },
		{ id: 'centered', name: 'Centered' },
		{ id: 'minimal', name: 'Minimal' }
	];
	const TEMPLATES = $derived(mode === 'rank' ? RANK_TEMPLATES : WELCOME_TEMPLATES);
	const ADD: { kind: ElKind; label: string }[] = $derived([
		{ kind: 'text', label: 'Text' },
		{ kind: 'avatar', label: 'Avatar' },
		{ kind: 'badge', label: 'Badge' },
		{ kind: 'divider', label: 'Divider' },
		{ kind: 'rect', label: 'Panel' },
		...(mode === 'rank' ? [{ kind: 'bar' as ElKind, label: 'XP bar' }] : [])
	]);

	const T0 = untrack(() =>
		mode === 'rank' ? (THEMES.find((t) => t.id === 'midnight') ?? THEMES[0]) : THEMES[0]
	);
	const INITIAL_TEMPLATE = untrack(() => (mode === 'rank' ? 'classic' : 'centered'));
	let themeId = $state(T0.id);
	let bg = $state<Background>({ ...T0.bg });
	const INITIAL = untrack(() => templateElements(INITIAL_TEMPLATE, T0, mode));
	let elements = $state<El[]>(INITIAL);
	let template = $state(INITIAL_TEMPLATE);
	let selectedId = $state<string | null>(INITIAL[0]?.id ?? null);

	const selIdx = $derived(elements.findIndex((e) => e.id === selectedId));
	const sel = $derived(selIdx >= 0 ? elements[selIdx] : null);
	const theme = $derived(THEMES.find((t) => t.id === themeId) ?? T0);

	function badgeText(accent: string, t = theme) {
		return accent === '#FFFFFF' ? t.bg.color || t.bg.to || '#1F1B2E' : '#FFFFFF';
	}

	function applyTemplate(id: string) {
		template = id;
		elements = templateElements(id, theme, mode);
		selectedId = elements[0]?.id ?? null;
	}

	function applyTheme(id: string) {
		const t = THEMES.find((x) => x.id === id);
		if (!t) return;
		themeId = id;
		bg = { ...t.bg };
		for (const el of elements) {
			if (el.role === 'title') el.color = t.text;
			else if (el.role === 'subtitle') el.color = t.subtext;
			else if (el.role === 'accent') {
				if (el.kind === 'badge') {
					el.bg = t.accent;
					el.color = badgeText(t.accent, t);
				} else el.color = t.accent;
			}
			if (el.kind === 'avatar') {
				el.ring = t.accent;
				el.color = t.text;
			}
		}
	}

	function addEl(kind: ElKind) {
		const t = theme;
		const common = {
			id: nextId(),
			x: 50,
			y: 50,
			anchor: 'center' as const,
			opacity: 1,
			rotation: 0,
			visible: true,
			locked: false
		};
		let el: El;
		if (kind === 'text')
			el = { ...common, kind, name: 'Text', role: 'custom', text: 'New text', font: 3.6, weight: 700, color: t.text, align: 'center', maxw: 80, letter: 0 };
		else if (kind === 'avatar')
			el = { ...common, kind, name: 'Avatar', role: 'custom', size: 18, ring: t.accent, shape: 'circle', color: t.text };
		else if (kind === 'badge')
			el = { ...common, kind, name: 'Badge', role: 'accent', text: 'Badge', font: 2.2, weight: 700, bg: t.accent, color: badgeText(t.accent) };
		else if (kind === 'divider')
			el = { ...common, kind, name: 'Divider', role: 'accent', w: 14, thickness: 3, color: t.accent };
		else if (kind === 'bar')
			el = { ...common, kind: 'bar', name: 'XP bar', role: 'accent', w: 60, height: 3.4, color: t.accent, track: 'rgba(255,255,255,0.18)', value: 62 };
		else el = { ...common, kind: 'rect', name: 'Panel', role: 'accent', w: 30, height: 40, radius: 14, color: t.accent, opacity: 0.18 };
		elements = [...elements, el];
		selectedId = el.id;
	}

	function removeEl(id: string) {
		const i = elements.findIndex((e) => e.id === id);
		elements = elements.filter((e) => e.id !== id);
		if (selectedId === id) selectedId = elements[Math.max(0, i - 1)]?.id ?? null;
	}
	function moveLayer(id: string, dir: 1 | -1) {
		const i = elements.findIndex((e) => e.id === id);
		const j = i + dir;
		if (i < 0 || j < 0 || j >= elements.length) return;
		const next = [...elements];
		[next[i], next[j]] = [next[j], next[i]];
		elements = next;
	}
	function reset() {
		applyTheme(mode === 'rank' ? 'midnight' : 'aurora');
		applyTemplate(INITIAL_TEMPLATE);
	}

	// Layers list shows top-most first.
	const layersTopFirst = $derived([...elements].reverse());
	const WEIGHTS = [400, 500, 600, 700, 800, 900];
</script>

<div class="card overflow-hidden">
	<!-- toolbar -->
	<div class="space-y-3 border-b border-line p-4">
		<div class="flex items-center justify-between">
			<div class="flex items-center gap-2">
				<h3 class="text-[15px] font-semibold">{mode === 'rank' ? 'Rank' : 'Welcome'} card editor</h3>
				<span class="rounded-md border border-line-strong px-2 py-0.5 font-mono text-[10px] uppercase tracking-[0.14em] text-faint">editable</span>
			</div>
			<button class="btn btn-ghost h-8 px-2.5 text-xs" onclick={reset}>
				<RotateCcw size={13} /> Reset
			</button>
		</div>

		<div>
			<span class="eyebrow mb-1.5 block">Layout</span>
			<div class="flex flex-wrap gap-1.5">
				{#each TEMPLATES as t (t.id)}
					<button
						type="button"
						aria-pressed={template === t.id}
						onclick={() => applyTemplate(t.id)}
						class="rounded-md border px-2.5 py-1 text-[13px] transition-colors {template === t.id
							? 'border-accent bg-blush text-accent-ink'
							: 'border-line-strong text-muted hover:text-ink'}"
					>
						{t.name}
					</button>
				{/each}
			</div>
		</div>

		<div>
			<span class="eyebrow mb-1.5 block">Theme</span>
			<div class="flex flex-wrap gap-2">
				{#each THEMES as t (t.id)}
					<button
						type="button"
						aria-label={t.name}
						aria-pressed={themeId === t.id}
						onclick={() => applyTheme(t.id)}
						title={t.name}
						class="h-7 w-7 rounded-full ring-offset-2 transition-all {themeId === t.id
							? 'ring-2 ring-accent'
							: 'ring-1 ring-line-strong hover:ring-ink'}"
						style="background:{t.bg.type === 'gradient'
							? `linear-gradient(135deg, ${t.bg.from}, ${t.bg.to})`
							: t.bg.color};"
					></button>
				{/each}
			</div>
		</div>
	</div>

	<!-- canvas stage -->
	<div class="bg-ink-2 p-4 sm:p-6">
		<div class="mx-auto max-w-2xl">
			<WelcomeCanvas
				interactive
				bind:selectedId
				{elements}
				{bg}
				aspect={mode === 'rank' ? '934 / 282' : '1024 / 450'}
				username="maya"
				count={1024}
				server="Aurora"
				level={12}
				rank={1}
				xp="4,820"
				nextxp="6,000"
				onnudgedelete={removeEl}
			/>
		</div>
		<p class="mt-2.5 text-center text-xs text-muted">
			Drag any element to move it · select one and use arrow keys to nudge
		</p>
	</div>

	<!-- panels -->
	<div class="grid border-t border-line lg:grid-cols-[210px_1fr]">
		<!-- layers -->
		<aside class="border-b border-line p-3 lg:border-b-0 lg:border-r">
			<div class="mb-2 flex items-center justify-between">
				<span class="eyebrow">Layers</span>
			</div>
			<div class="mb-3 flex flex-wrap gap-1">
				{#each ADD as a (a.kind)}
					<button
						type="button"
						onclick={() => addEl(a.kind)}
						class="inline-flex items-center gap-1 rounded-md border border-line-strong px-1.5 py-1 text-[11px] font-medium text-muted transition-colors hover:text-ink"
					>
						<Plus size={11} />{a.label}
					</button>
				{/each}
			</div>
			<ul class="space-y-0.5">
				{#each layersTopFirst as el (el.id)}
					<li>
						<div
							class="group flex items-center gap-1 rounded-md px-1.5 py-1 {selectedId === el.id
								? 'bg-blush'
								: 'hover:bg-ink-2'}"
						>
							<button
								type="button"
								onclick={() => (el.visible = !el.visible)}
								class="text-faint hover:text-ink"
								aria-label={el.visible ? 'Hide' : 'Show'}
							>
								{#if el.visible}<Eye size={13} />{:else}<EyeOff size={13} />{/if}
							</button>
							<button
								type="button"
								onclick={() => (selectedId = el.id)}
								class="flex-1 truncate text-left text-[13px] {selectedId === el.id
									? 'font-medium text-accent-ink'
									: 'text-ink'}"
							>
								{el.name}
							</button>
							<button type="button" onclick={() => moveLayer(el.id, 1)} class="text-faint opacity-0 transition-opacity hover:text-ink focus-visible:opacity-100 group-hover:opacity-100 group-focus-within:opacity-100" aria-label="Bring forward">
								<ChevronUp size={13} />
							</button>
							<button type="button" onclick={() => moveLayer(el.id, -1)} class="text-faint opacity-0 transition-opacity hover:text-ink focus-visible:opacity-100 group-hover:opacity-100 group-focus-within:opacity-100" aria-label="Send back">
								<ChevronDown size={13} />
							</button>
							<button type="button" onclick={() => removeEl(el.id)} class="text-faint opacity-0 transition-opacity hover:text-danger focus-visible:opacity-100 group-hover:opacity-100 group-focus-within:opacity-100" aria-label="Delete">
								<Trash2 size={12} />
							</button>
						</div>
					</li>
				{/each}
			</ul>
			<button
				type="button"
				onclick={() => (selectedId = null)}
				class="mt-2 w-full rounded-md border px-2 py-1.5 text-left text-[13px] transition-colors {selectedId ===
				null
					? 'border-accent bg-blush text-accent-ink'
					: 'border-line-strong text-muted hover:text-ink'}"
			>
				Background
			</button>
		</aside>

		<!-- inspector -->
		<section class="min-w-0 p-4">
			{#if sel}
				<div class="mb-3 flex items-center justify-between">
					<span class="text-[13px] font-semibold capitalize">{sel.kind} — {sel.name}</span>
					<button class="text-faint hover:text-danger" onclick={() => removeEl(sel.id)} aria-label="Delete element">
						<Trash2 size={14} />
					</button>
				</div>

				<!-- common: position -->
				<div class="grid grid-cols-2 gap-3">
					<label class="block">
						<span class="hint mb-1 block">X · {Math.round(elements[selIdx].x)}%</span>
						<input type="range" min="0" max="100" step="0.5" bind:value={elements[selIdx].x} class="w-full accent-[#ff6363]" />
					</label>
					<label class="block">
						<span class="hint mb-1 block">Y · {Math.round(elements[selIdx].y)}%</span>
						<input type="range" min="0" max="100" step="0.5" bind:value={elements[selIdx].y} class="w-full accent-[#ff6363]" />
					</label>
				</div>

				<!-- anchor -->
				<div class="mt-3">
					<span class="hint mb-1 block">Anchor</span>
					<div class="inline-flex rounded-lg border border-line-strong p-0.5">
						{#each ['left', 'center', 'right'] as a (a)}
							<button
								type="button"
								onclick={() => (elements[selIdx].anchor = a as 'left' | 'center' | 'right')}
								aria-pressed={sel.anchor === a}
								class="rounded-md px-2.5 py-1 text-xs capitalize {sel.anchor === a
									? 'bg-ink text-bg'
									: 'text-muted'}">{a}</button
							>
						{/each}
					</div>
				</div>

				<!-- text / badge content -->
				{#if sel.kind === 'text' || sel.kind === 'badge'}
					<label class="mt-3 block">
						<span class="label">Text</span>
						<input class="input" bind:value={elements[selIdx].text} />
					</label>
					<div class="mt-3 grid grid-cols-2 gap-3">
						<label class="block">
							<span class="hint mb-1 block">Size · {sel.font?.toFixed(1)}</span>
							<input type="range" min="1.5" max="11" step="0.1" bind:value={elements[selIdx].font} class="w-full accent-[#ff6363]" />
						</label>
						<label class="block">
							<span class="label">Weight</span>
							<select class="input" bind:value={elements[selIdx].weight}>
								{#each WEIGHTS as w (w)}<option value={w}>{w}</option>{/each}
							</select>
						</label>
					</div>
					{#if sel.kind === 'text'}
						<div class="mt-3">
							<span class="hint mb-1 block">Align</span>
							<div class="inline-flex rounded-lg border border-line-strong p-0.5">
								{#each ['left', 'center', 'right'] as a (a)}
									<button type="button" onclick={() => (elements[selIdx].align = a as 'left' | 'center' | 'right')} aria-pressed={sel.align === a} class="rounded-md px-2.5 py-1 text-xs capitalize {sel.align === a ? 'bg-ink text-bg' : 'text-muted'}">{a}</button>
								{/each}
							</div>
						</div>
					{/if}
					<div class="mt-3"><ColorField label="Text colour" bind:value={elements[selIdx].color} /></div>
					{#if sel.kind === 'badge'}
						<div class="mt-3"><ColorField label="Background" bind:value={elements[selIdx].bg} /></div>
					{/if}
				{/if}

				<!-- avatar -->
				{#if sel.kind === 'avatar'}
					<label class="mt-3 block">
						<span class="hint mb-1 block">Size · {Math.round(sel.size ?? 0)}%</span>
						<input type="range" min="8" max="40" step="0.5" bind:value={elements[selIdx].size} class="w-full accent-[#ff6363]" />
					</label>
					<div class="mt-3">
						<span class="hint mb-1 block">Shape</span>
						<div class="inline-flex rounded-lg border border-line-strong p-0.5">
							{#each ['circle', 'rounded', 'square'] as s (s)}
								<button type="button" onclick={() => (elements[selIdx].shape = s as 'circle' | 'rounded' | 'square')} aria-pressed={sel.shape === s} class="rounded-md px-2.5 py-1 text-xs capitalize {sel.shape === s ? 'bg-ink text-bg' : 'text-muted'}">{s}</button>
							{/each}
						</div>
					</div>
					<div class="mt-3"><ColorField label="Ring colour" bind:value={elements[selIdx].ring} /></div>
				{/if}

				<!-- rect -->
				{#if sel.kind === 'rect'}
					<div class="mt-3 grid grid-cols-2 gap-3">
						<label class="block"><span class="hint mb-1 block">Width · {Math.round(sel.w ?? 0)}</span><input type="range" min="5" max="100" step="1" bind:value={elements[selIdx].w} class="w-full accent-[#ff6363]" /></label>
						<label class="block"><span class="hint mb-1 block">Height · {Math.round(sel.height ?? 0)}</span><input type="range" min="2" max="120" step="1" bind:value={elements[selIdx].height} class="w-full accent-[#ff6363]" /></label>
						<label class="block"><span class="hint mb-1 block">Radius · {sel.radius}px</span><input type="range" min="0" max="60" step="1" bind:value={elements[selIdx].radius} class="w-full accent-[#ff6363]" /></label>
					</div>
					<div class="mt-3"><ColorField label="Colour" bind:value={elements[selIdx].color} /></div>
				{/if}

				<!-- divider -->
				{#if sel.kind === 'divider'}
					<div class="mt-3 grid grid-cols-2 gap-3">
						<label class="block"><span class="hint mb-1 block">Width · {Math.round(sel.w ?? 0)}</span><input type="range" min="2" max="100" step="1" bind:value={elements[selIdx].w} class="w-full accent-[#ff6363]" /></label>
						<label class="block"><span class="hint mb-1 block">Thickness · {sel.thickness}px</span><input type="range" min="1" max="20" step="1" bind:value={elements[selIdx].thickness} class="w-full accent-[#ff6363]" /></label>
					</div>
					<div class="mt-3"><ColorField label="Colour" bind:value={elements[selIdx].color} /></div>
				{/if}

				{#if sel.kind === 'bar'}
					<div class="mt-3 grid grid-cols-2 gap-3">
						<label class="block"><span class="hint mb-1 block">Width · {Math.round(sel.w ?? 0)}</span><input type="range" min="10" max="100" step="1" bind:value={elements[selIdx].w} class="w-full accent-[#ff6363]" /></label>
						<label class="block"><span class="hint mb-1 block">Fill · {Math.round(sel.value ?? 0)}%</span><input type="range" min="0" max="100" step="1" bind:value={elements[selIdx].value} class="w-full accent-[#ff6363]" /></label>
						<label class="block"><span class="hint mb-1 block">Thickness · {sel.height?.toFixed(1)}</span><input type="range" min="1" max="10" step="0.2" bind:value={elements[selIdx].height} class="w-full accent-[#ff6363]" /></label>
					</div>
					<div class="mt-3 grid grid-cols-2 gap-3">
						<ColorField label="Fill" bind:value={elements[selIdx].color} />
						<ColorField label="Track" bind:value={elements[selIdx].track} />
					</div>
				{/if}

				<!-- common: opacity + rotation -->
				<div class="mt-3 grid grid-cols-2 gap-3">
					<label class="block"><span class="hint mb-1 block">Opacity · {Math.round((sel.opacity ?? 1) * 100)}%</span><input type="range" min="0" max="1" step="0.05" bind:value={elements[selIdx].opacity} class="w-full accent-[#ff6363]" /></label>
					<label class="block"><span class="hint mb-1 block">Rotation · {sel.rotation}°</span><input type="range" min="-45" max="45" step="1" bind:value={elements[selIdx].rotation} class="w-full accent-[#ff6363]" /></label>
				</div>
			{:else}
				<!-- background inspector -->
				<div class="mb-3 text-[13px] font-semibold">Background</div>
				<div class="inline-flex rounded-lg border border-line-strong p-0.5">
					{#each ['gradient', 'solid', 'image'] as t (t)}
						<button type="button" onclick={() => (bg.type = t as 'gradient' | 'solid' | 'image')} aria-pressed={bg.type === t} class="rounded-md px-2.5 py-1 text-xs capitalize {bg.type === t ? 'bg-ink text-bg' : 'text-muted'}">{t}</button>
					{/each}
				</div>
				{#if bg.type === 'gradient'}
					<div class="mt-3 grid grid-cols-2 gap-3">
						<ColorField label="From" bind:value={bg.from} />
						<ColorField label="To" bind:value={bg.to} />
					</div>
					<label class="mt-3 block">
						<span class="hint mb-1 block">Angle · {bg.angle}°</span>
						<input type="range" min="0" max="360" step="1" bind:value={bg.angle} class="w-full accent-[#ff6363]" />
					</label>
				{:else if bg.type === 'solid'}
					<div class="mt-3"><ColorField label="Colour" bind:value={bg.color} /></div>
				{:else}
					<label class="mt-3 block">
						<span class="label">Image URL</span>
						<input class="input" placeholder="https://image.png" bind:value={bg.image} />
					</label>
				{/if}
				<p class="hint mt-3">Tip: pick a layer on the left to style individual elements.</p>
			{/if}
		</section>
	</div>
</div>
