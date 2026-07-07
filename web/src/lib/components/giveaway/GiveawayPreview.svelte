<script lang="ts">
	// A live, Discord-styled preview of the composed giveaway message: the
	// content line, the embed(s), and the system Enter button. Template
	// variables ({{ .Prize }}, {{ .Ends }}, …) are filled in client-side from a
	// sample scope so the panel updates instantly as you type. It is a visual
	// approximation (Go template logic isn't executed here — the per-field "Test
	// render" runs the real engine); its job is to show layout, colour and copy.
	import type { EmbedSpec, ButtonConfig } from '$lib/giveaway';

	let {
		content = '',
		embeds = [],
		color = '',
		imageUrl = '',
		button,
		pingRoleName = '',
		showRequirements = false,
		requirementsSummary = '',
		sample = {}
	}: {
		content?: string;
		embeds?: EmbedSpec[];
		color?: string;
		imageUrl?: string;
		button: ButtonConfig;
		pingRoleName?: string;
		showRequirements?: boolean;
		requirementsSummary?: string;
		sample?: Record<string, unknown>;
	} = $props();

	// fill substitutes the known {{ .Var }} tokens, then strips any remaining
	// template tags (logic like {{ if … }}) so the preview reads cleanly.
	function fill(s: string | undefined): string {
		if (!s) return '';
		return s
			.replace(/\{\{\s*\.(\w+)\s*\}\}/g, (_m, key) => {
				const v = sample[key];
				return v === undefined ? '' : String(v);
			})
			.replace(/\{\{[^}]*\}\}/g, '')
			.trim();
	}

	const BRAND = '#FF6363';
	function hex(v: string): string {
		const h = (v || '').trim();
		return /^#?[0-9a-fA-F]{6}$/.test(h) ? (h.startsWith('#') ? h : `#${h}`) : '';
	}
	const accent = $derived(hex(color) || hex(embeds[0]?.color ?? '') || BRAND);

	// Discord's real button colours, so the Enter button reads as it will in-app.
	const btnColors: Record<string, string> = {
		primary: '#5865f2',
		success: '#248046',
		danger: '#da373c',
		secondary: '#4e5058'
	};
	const btnBg = $derived(btnColors[button?.style] ?? btnColors.primary);
	// A unicode glyph shows as-is; a custom "name:id" can't render here, so skip it.
	const btnEmoji = $derived(button?.emoji && !button.emoji.includes(':') ? button.emoji : '');

	type Field = { name: string; value: string; inline?: boolean };
	function fieldsOf(e: EmbedSpec): Field[] {
		const out = (e.fields ?? []).map((f) => ({
			name: fill(f.name),
			value: fill(f.value),
			inline: f.inline
		}));
		if (showRequirements && requirementsSummary && e === embeds[0]) {
			out.push({ name: 'Requirements', value: requirementsSummary, inline: false });
		}
		return out.filter((f) => f.name || f.value);
	}

	const filledContent = $derived(fill(content));
</script>

<div class="rounded-lg border border-line bg-[#313338] p-3 text-[#dbdee1] shadow-sm">
	<!-- Bot author row -->
	<div class="mb-1.5 flex items-center gap-2">
		<span class="grid size-6 place-items-center rounded-full bg-accent text-[10px] font-bold text-white">D</span>
		<span class="text-[13px] font-medium text-white">Dia</span>
		<span class="rounded bg-accent px-1 py-0.5 text-[9px] font-semibold uppercase tracking-wide text-white">bot</span>
	</div>

	{#if pingRoleName}
		<div class="mb-1 text-[13px]">
			<span class="rounded bg-accent/25 px-1 font-medium text-accent-ink">@{pingRoleName}</span>
		</div>
	{/if}
	{#if filledContent}
		<div class="mb-1.5 text-[13px] leading-relaxed whitespace-pre-wrap">{filledContent}</div>
	{/if}

	{#each embeds as e, i (i)}
		{@const title = fill(e.title)}
		{@const desc = fill(e.description)}
		{@const fields = fieldsOf(e)}
		{@const emColor = hex(color) || hex(e.color ?? '') || BRAND}
		{@const img = fill(e.image_url) || (i === 0 ? imageUrl : '')}
		<div
			class="mt-1 max-w-md overflow-hidden rounded-[4px] bg-[#2b2d31]"
			style="border-left: 4px solid {emColor}"
		>
			<div class="px-3 py-2.5">
				{#if fill(e.author_name)}
					<div class="mb-1 text-[12px] font-medium text-white">{fill(e.author_name)}</div>
				{/if}
				{#if title}
					<div class="text-[14px] font-semibold text-white">{title}</div>
				{/if}
				{#if desc}
					<div class="mt-1 text-[13px] leading-relaxed whitespace-pre-wrap text-[#dbdee1]">{desc}</div>
				{/if}
				{#if fields.length}
					<div class="mt-2 flex flex-wrap gap-x-4 gap-y-2">
						{#each fields as f (f.name + f.value)}
							<div class={f.inline ? 'min-w-[30%] flex-1' : 'w-full'}>
								{#if f.name}<div class="text-[12px] font-semibold text-white">{f.name}</div>{/if}
								{#if f.value}<div class="text-[13px] whitespace-pre-wrap text-[#dbdee1]">{f.value}</div>{/if}
							</div>
						{/each}
					</div>
				{/if}
				{#if img}
					<img src={img} alt="" class="mt-2 max-h-48 w-full rounded object-cover" />
				{/if}
				{#if fill(e.footer_text)}
					<div class="mt-2 text-[11px] text-[#949ba4]">{fill(e.footer_text)}</div>
				{/if}
			</div>
		</div>
	{/each}

	<!-- The system-managed Enter button -->
	<div class="mt-2">
		<span
			class="inline-flex items-center gap-1.5 rounded-[3px] px-3 py-1.5 text-[13px] font-medium text-white"
			style="background-color: {btnBg}"
		>
			{#if btnEmoji}<span>{btnEmoji}</span>{/if}
			{button?.label?.trim() || 'Enter Giveaway'}
		</span>
	</div>
</div>
