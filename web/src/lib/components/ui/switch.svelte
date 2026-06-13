<script lang="ts">
	import { Switch as SwitchPrimitive } from 'bits-ui';
	import { cn } from '$lib/utils';

	let {
		checked = $bindable(false),
		class: cls,
		disabled = false,
		onCheckedChange
	}: {
		checked?: boolean;
		class?: string;
		disabled?: boolean;
		onCheckedChange?: (next: boolean) => void;
	} = $props();
</script>

<SwitchPrimitive.Root
	bind:checked
	{disabled}
	{onCheckedChange}
	class={cn(
		'peer inline-flex h-4 w-7 shrink-0 cursor-pointer items-center rounded-full border border-transparent transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:bg-primary data-[state=unchecked]:bg-secondary',
		cls
	)}
>
	<SwitchPrimitive.Thumb
		class="pointer-events-none block h-3 w-3 rounded-full bg-background shadow-sm ring-0 transition-transform data-[state=checked]:translate-x-3.5 data-[state=unchecked]:translate-x-0.5"
	/>
</SwitchPrimitive.Root>
