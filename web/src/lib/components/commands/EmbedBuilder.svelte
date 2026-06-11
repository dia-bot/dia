<script lang="ts">
	// The central embed component: a Discord-accurate embed you edit IN PLACE.
	// Title, description, author, fields and footer are typed directly into the
	// embed; the accent bar is the color picker; image / thumbnail / icons are
	// clickable slots with real picture upload. Mirrors EmbedSpec (kinds.go) —
	// every text value is a Go template rendered at runtime.
	import ImagePicker from './ImagePicker.svelte';

	import Link2 from 'lucide-svelte/icons/link-2';
	import Plus from 'lucide-svelte/icons/plus';
	import X from 'lucide-svelte/icons/x';
	import Trash2 from 'lucide-svelte/icons/trash-2';
	import Clock from 'lucide-svelte/icons/clock';

	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	type AnyEmbed = any;

	let {
		embed,
		onChange,
		onRemove
	}: {
		embed: AnyEmbed;
		onChange: (next: AnyEmbed) => void;
		onRemove?: () => void;
	} = $props();

	function set(field: string, value: unknown) {
		const next = { ...(embed ?? {}) };
		if (value === '' || value === false || value === undefined) delete next[field];
		else next[field] = value;
		onChange(next);
	}

	const fields = $derived(
		(embed?.fields ?? []) as { name: string; value: string; inline?: boolean }[]
	);

	function setField(i: number, patch: Partial<{ name: string; value: string; inline: boolean }>) {
		onChange({
			...(embed ?? {}),
			fields: fields.map((f, idx) => (idx === i ? { ...f, ...patch } : f))
		});
	}
	function addField() {
		if (fields.length >= 25) return;
		onChange({ ...(embed ?? {}), fields: [...fields, { name: '', value: '' }] });
	}
	function removeField(i: number) {
		const next = fields.filter((_, idx) => idx !== i);
		const e = { ...(embed ?? {}) };
		if (next.length) e.fields = next;
		else delete e.fields;
		onChange(e);
	}

	const color = $derived((embed?.color as string) ?? '');
	const validColor = $derived(/^#[0-9a-fA-F]{6}$/.test(color));

	let urlOpen = $state(false);

	// Grow textareas with their content so the embed reads like the real thing.
	function autogrow(el: HTMLTextAreaElement) {
		const fit = () => {
			el.style.height = '0';
			el.style.height = `${el.scrollHeight}px`;
		};
		fit();
		el.addEventListener('input', fit);
		return { destroy: () => el.removeEventListener('input', fit) };
	}
</script>

<div class="dc-embed group/embed relative flex overflow-hidden rounded-[4px] bg-[#2b2d31]">
	<!-- The accent bar IS the color picker. -->
	<label
		class="relative w-1.5 shrink-0 cursor-pointer transition-[filter] hover:brightness-125"
		style="background: {validColor ? color : '#1e1f22'}"
		title="Accent color"
	>
		<input
			type="color"
			class="absolute inset-0 size-full cursor-pointer opacity-0"
			value={validColor ? color : '#5865f2'}
			oninput={(e) => set('color', (e.currentTarget as HTMLInputElement).value)}
		/>
	</label>

	<div class="min-w-0 flex-1 px-3.5 pb-3.5 pt-2.5">
		<div class="flex gap-3.5">
			<div class="min-w-0 flex-1">
				<!-- Author -->
				<div class="mb-0.5 flex items-center gap-2">
					<ImagePicker
						shape="icon"
						label="Author icon"
						value={embed?.author_icon ?? ''}
						onChange={(v) => set('author_icon', v)}
					/>
					<input
						class="dc-input h-5 min-w-0 flex-1 text-[12.5px] font-semibold text-[#f2f3f5]"
						placeholder="Author"
						maxlength="256"
						value={embed?.author_name ?? ''}
						oninput={(e) => set('author_name', (e.currentTarget as HTMLInputElement).value)}
					/>
				</div>

				<!-- Title + link -->
				<div class="flex items-center gap-1.5">
					<input
						class="dc-input min-w-0 flex-1 py-0.5 text-[15px] font-semibold {embed?.url
							? 'text-[#00a8fc]'
							: 'text-[#f2f3f5]'}"
						placeholder="Title"
						maxlength="256"
						value={embed?.title ?? ''}
						oninput={(e) => set('title', (e.currentTarget as HTMLInputElement).value)}
					/>
					<button
						type="button"
						class="grid size-5 shrink-0 place-items-center rounded transition-colors {embed?.url || urlOpen
							? 'text-[#00a8fc]'
							: 'text-[#6d6f78] opacity-0 hover:text-[#b5bac1] group-hover/embed:opacity-100'}"
						title="Link the title"
						onclick={() => (urlOpen = !urlOpen)}
					>
						<Link2 size={12} />
					</button>
				</div>
				{#if urlOpen || embed?.url}
					<input
						class="dc-input mb-1 w-full font-mono text-[10.5px] text-[#00a8fc]"
						placeholder="https://… title link"
						value={embed?.url ?? ''}
						oninput={(e) => set('url', (e.currentTarget as HTMLInputElement).value)}
					/>
				{/if}

				<!-- Description -->
				<textarea
					use:autogrow
					rows="1"
					class="dc-input w-full resize-none py-0.5 text-[12.5px] leading-[1.4] text-[#dbdee1]"
					placeholder="Description — Go templates and markdown work here"
					maxlength="4096"
					value={embed?.description ?? ''}
					oninput={(e) => set('description', (e.currentTarget as HTMLTextAreaElement).value)}
				></textarea>

				<!-- Fields — laid out like Discord lays them out -->
				{#if fields.length > 0}
					<div class="mt-1.5 flex flex-wrap gap-x-3 gap-y-1.5">
						{#each fields as f, i (i)}
							<div class="group/field min-w-0 {f.inline ? 'basis-[30%] grow' : 'basis-full'}">
								<!-- Name row carries the field's controls — nothing floats over it. -->
								<div class="flex items-center gap-1">
									<input
										class="dc-input min-w-0 flex-1 text-[12px] font-semibold text-[#f2f3f5]"
										placeholder="Field name"
										maxlength="256"
										value={f.name}
										oninput={(e) => setField(i, { name: (e.currentTarget as HTMLInputElement).value })}
									/>
									<button
										type="button"
										class="shrink-0 rounded px-1 py-px font-mono text-[8.5px] uppercase tracking-[0.08em] transition-all {f.inline
											? 'bg-white/10 text-[#dbdee1]'
											: 'text-[#6d6f78] opacity-0 hover:text-[#b5bac1] group-focus-within/field:opacity-100 group-hover/field:opacity-100'}"
										title="Side by side with neighbouring inline fields"
										onclick={() => setField(i, { inline: !f.inline })}
									>
										inline
									</button>
									<button
										type="button"
										class="grid size-4 shrink-0 place-items-center rounded text-[#6d6f78] opacity-0 transition-all hover:text-[#fa777c] group-focus-within/field:opacity-100 group-hover/field:opacity-100"
										onclick={() => removeField(i)}
										aria-label="Remove field"
									>
										<X size={9} />
									</button>
								</div>
								<textarea
									use:autogrow
									rows="1"
									class="dc-input w-full resize-none text-[12px] leading-[1.4] text-[#dbdee1]"
									placeholder="Value"
									maxlength="1024"
									value={f.value}
									oninput={(e) =>
										setField(i, { value: (e.currentTarget as HTMLTextAreaElement).value })}
								></textarea>
							</div>
						{/each}
					</div>
				{/if}
				<button
					type="button"
					class="mt-1 inline-flex h-5 items-center gap-1 rounded text-[10.5px] font-medium text-[#6d6f78] opacity-0 transition-all hover:text-[#b5bac1] group-hover/embed:opacity-100 disabled:opacity-0"
					onclick={addField}
					disabled={fields.length >= 25}
				>
					<Plus size={10} />
					field
				</button>

				<!-- Large image -->
				<div class="mt-2">
					<ImagePicker
						shape="banner"
						label="Image"
						value={embed?.image_url ?? ''}
						onChange={(v) => set('image_url', v)}
					/>
				</div>

				<!-- Footer -->
				<div class="mt-2 flex items-center gap-1.5">
					<ImagePicker
						shape="icon"
						label="Footer icon"
						value={embed?.footer_icon ?? ''}
						onChange={(v) => set('footer_icon', v)}
					/>
					<input
						class="dc-input h-5 min-w-0 flex-1 text-[11px] font-medium text-[#949ba4]"
						placeholder="Footer"
						maxlength="2048"
						value={embed?.footer_text ?? ''}
						oninput={(e) => set('footer_text', (e.currentTarget as HTMLInputElement).value)}
					/>
					<button
						type="button"
						class="inline-flex h-5 shrink-0 items-center gap-1 rounded px-1 text-[10px] font-medium transition-colors {embed?.timestamp
							? 'text-[#dbdee1]'
							: 'text-[#6d6f78] opacity-0 hover:text-[#b5bac1] group-hover/embed:opacity-100'}"
						title="Stamp with the send time"
						onclick={() => set('timestamp', !embed?.timestamp)}
					>
						<Clock size={10} />
						{embed?.timestamp ? 'today at 4:20 PM' : 'timestamp'}
					</button>
				</div>
			</div>

			<!-- Thumbnail column -->
			<div class="shrink-0 pt-1">
				<ImagePicker
					shape="thumb"
					label="Thumbnail"
					value={embed?.thumbnail ?? ''}
					onChange={(v) => set('thumbnail', v)}
				/>
			</div>
		</div>
	</div>

	<!-- Hover utilities: clear color / remove embed -->
	<div class="absolute right-1.5 top-1.5 flex items-center gap-0.5 opacity-0 transition-opacity group-hover/embed:opacity-100">
		{#if validColor}
			<button
				type="button"
				class="rounded bg-[#1e1f22] px-1.5 py-0.5 font-mono text-[8.5px] uppercase tracking-[0.08em] text-[#949ba4] transition-colors hover:text-[#f2f3f5]"
				onclick={() => set('color', '')}
			>
				clear color
			</button>
		{/if}
		{#if onRemove}
			<button
				type="button"
				class="grid size-5 place-items-center rounded bg-[#1e1f22] text-[#949ba4] transition-colors hover:text-[#fa777c]"
				onclick={onRemove}
				title="Remove embed"
				aria-label="Remove embed"
			>
				<Trash2 size={10} />
			</button>
		{/if}
	</div>
</div>

<style>
	.dc-input {
		background: transparent;
		border: none;
		outline: none;
		border-radius: 3px;
	}
	.dc-input::placeholder {
		color: #6d6f78;
		font-weight: 400;
	}
	.dc-input:hover {
		background: rgba(255, 255, 255, 0.03);
	}
	.dc-input:focus {
		background: rgba(255, 255, 255, 0.05);
	}
</style>
