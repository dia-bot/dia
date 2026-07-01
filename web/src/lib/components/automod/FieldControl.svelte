<script lang="ts">
	// Generic control for one FieldSpec, bound to obj[spec.key]. The parent decides
	// whether to render it (showWhen) by passing only visible specs; this component
	// just maps FieldSpec.type -> control. obj is a reactive $state record so writes
	// propagate back to the rule/action. Function bindings keep the casts local.
	import type { FieldSpec } from '$lib/moderation/automod';
	import NumberField from '$lib/components/ui/NumberField.svelte';
	import Select from '$lib/components/Select.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import RolePicker from '$lib/components/RolePicker.svelte';
	import ChannelPicker from '$lib/components/ChannelPicker.svelte';
	import ChipInput from './ChipInput.svelte';
	import DurationField from './DurationField.svelte';
	import AutomationPicker from '$lib/components/commands/AutomationPicker.svelte';

	let { spec, obj }: { spec: FieldSpec; obj: Record<string, unknown> } = $props();

	const str = () => (obj[spec.key] as string) ?? '';
	const setStr = (v: string) => (obj[spec.key] = v);
	const numv = () => obj[spec.key] as number | undefined;
	const setNum = (v: number | undefined) => (obj[spec.key] = v);
	const list = () => (Array.isArray(obj[spec.key]) ? (obj[spec.key] as string[]) : []);
	const setList = (v: string[]) => (obj[spec.key] = v);
</script>

{#if spec.type === 'toggle'}
	<div class="flex items-center justify-between gap-4">
		<div>
			<span class="text-sm font-medium text-ink">{spec.label}</span>
			{#if spec.hint}<p class="text-xs text-muted">{spec.hint}</p>{/if}
		</div>
		<Toggle checked={!!obj[spec.key]} label={spec.label} onchange={(v) => (obj[spec.key] = v)} />
	</div>
{:else}
	<div class="space-y-1.5">
		<span class="label !mb-0">{spec.label}</span>

		{#if spec.type === 'text'}
			<input
				class="input"
				placeholder={spec.placeholder ?? ''}
				value={str()}
				oninput={(e) => setStr(e.currentTarget.value)}
			/>
		{:else if spec.type === 'textarea'}
			<textarea
				class="input"
				rows="3"
				placeholder={spec.placeholder ?? ''}
				value={str()}
				oninput={(e) => setStr(e.currentTarget.value)}
			></textarea>
		{:else if spec.type === 'number'}
			<div class="flex items-center gap-2">
				<div class="w-32">
					<NumberField bind:value={numv, setNum} min={spec.min} max={spec.max} />
				</div>
				{#if spec.suffix}<span class="text-xs text-muted">{spec.suffix}</span>{/if}
			</div>
		{:else if spec.type === 'duration'}
			<DurationField bind:value={numv, setNum} />
		{:else if spec.type === 'select'}
			<Select bind:value={str, setStr} options={spec.options ?? []} />
		{:else if spec.type === 'words'}
			<ChipInput bind:value={list, setList} placeholder={spec.placeholder ?? 'Add a term…'} />
		{:else if spec.type === 'role'}
			<RolePicker value={str()} onChange={(v) => setStr(v as string)} placeholder="Select a role…" />
		{:else if spec.type === 'channel'}
			<ChannelPicker
				value={str()}
				onChange={(v) => setStr(v as string)}
				placeholder="Select a channel…"
			/>
		{:else if spec.type === 'automation'}
			<AutomationPicker value={str()} onChange={(v) => setStr(v)} />
		{/if}

		{#if spec.hint}<p class="hint !mt-0">{spec.hint}</p>{/if}
	</div>
{/if}
