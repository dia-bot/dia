<script lang="ts">
	// Channel field with three modes: the channel the command ran in, a real
	// channel picked from the server, or a template for dynamic flows. Stores
	// a plain Expr either way, so the runtime sees nothing new.
	import type { Expr } from '$lib/commands/types';
	import ChannelSelect from '$lib/components/ChannelSelect.svelte';
	import ExprField from './ExprField.svelte';
	import FieldSelect from './FieldSelect.svelte';

	let {
		value,
		onChange
	}: {
		value: Expr | undefined;
		onChange: (v: Expr) => void;
	} = $props();

	type Mode = 'here' | 'pick' | 'tmpl';

	function modeOf(src: string): Mode {
		if (src === '' || src === '{channel.id}') return 'here';
		if (/^\d{15,21}$/.test(src)) return 'pick';
		return 'tmpl';
	}

	// Mode follows the stored value, but an explicit user choice wins (so
	// switching to "pick" with nothing selected yet doesn't snap back).
	let chosen = $state<Mode | null>(null);
	const src = $derived(value?.src ?? '');
	const mode = $derived(chosen ?? modeOf(src));

	function setMode(m: Mode) {
		chosen = m;
		if (m === 'here') onChange({ lang: 'tmpl', src: '{channel.id}' });
		else if (m === 'pick' && !/^\d{15,21}$/.test(src)) onChange({ lang: 'tmpl', src: '' });
	}

	// ChannelSelect binds internally; mirror picks back into the Expr.
	let picked = $state('');
	$effect(() => {
		picked = /^\d{15,21}$/.test(src) ? src : '';
	});
	$effect(() => {
		if (mode === 'pick' && picked && picked !== src) onChange({ lang: 'tmpl', src: picked });
	});
</script>

<div class="space-y-1.5">
	<FieldSelect
		value={mode}
		onChange={(v) => setMode(v as Mode)}
		options={[
			{ value: 'here', label: 'The channel the command ran in' },
			{ value: 'pick', label: 'A specific channel' },
			{ value: 'tmpl', label: 'From a template / variable' }
		]}
	/>
	{#if mode === 'pick'}
		<ChannelSelect bind:value={picked} placeholder="Pick a channel…" />
	{:else if mode === 'tmpl'}
		<ExprField {value} {onChange} placeholder={'{{ .Vars.thread.id }}'} />
	{/if}
</div>
