<script lang="ts">
	// Single-channel select. Thin wrapper over the central ChannelPicker so the
	// existing `bind:value` call sites keep working while everyone shares one UI.
	import ChannelPicker from './ChannelPicker.svelte';

	let {
		value = $bindable(''),
		placeholder = 'Select a channel…',
		kind = 'text'
	}: { value?: string; placeholder?: string; kind?: 'text' | 'voice' | 'all' } = $props();
</script>

<ChannelPicker {value} {placeholder} {kind} onChange={(v) => (value = v as string)} />
