<script lang="ts">
	// Renders an emoji string the way Discord would: unicode emojis as text,
	// custom server emojis ("name:id" / "a:name:id" / pasted "<a:name:id>")
	// as their real CDN image. Used everywhere an emoji value is previewed.
	let {
		emoji = '',
		size = 16
	}: {
		emoji?: string;
		size?: number;
	} = $props();

	const custom = $derived(
		/^(a:)?([\w~-]+):(\d{15,21})$/.exec((emoji ?? '').trim().replace(/^<|>$/g, ''))
	);
</script>

{#if custom}
	<img
		src={`https://cdn.discordapp.com/emojis/${custom[3]}.${custom[1] ? 'gif' : 'png'}?size=64`}
		alt={custom[2]}
		style="width: {size}px; height: {size}px"
		class="inline-block shrink-0 object-contain"
	/>
{:else if emoji}
	<span class="shrink-0 leading-none" style="font-size: {size - 2}px">{emoji}</span>
{/if}
