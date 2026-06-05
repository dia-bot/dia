<script lang="ts">
	let {
		size = 28,
		wordmark = false,
		tile = false,
		dark = false
	}: { size?: number; wordmark?: boolean; tile?: boolean; dark?: boolean } = $props();
</script>

<span class="inline-flex items-center gap-2.5">
	<svg width={size} height={size} viewBox="0 0 512 512" fill="none" role="img" aria-label="Dia">
		{#if tile}<rect width="512" height="512" rx="120" fill="#F1DFDF" />{/if}
		<path
			d="M138.903 195.097C92.1029 112.297 138.503 100.763 169.17 102.097C278.837 125.43 412.174 195.464 391.374 338.664C365.374 517.664 320.995 293.274 239.403 375.597C153.67 462.097 197.403 298.597 138.903 195.097Z"
			fill="url(#diaLogoGrad)"
		/>
		<defs>
			<linearGradient
				id="diaLogoGrad"
				x1="214.228"
				y1="89.5966"
				x2="394"
				y2="450"
				gradientUnits="userSpaceOnUse"
			>
				<stop stop-color="#FF6363" />
				<stop offset="1" stop-color="#B244FC" />
			</linearGradient>
		</defs>
	</svg>
	{#if wordmark}
		<span class="font-extrabold tracking-tight {dark ? 'text-white' : 'text-ink'}" style="font-size: {size * 0.62}px">Dia</span>
	{/if}
</span>
