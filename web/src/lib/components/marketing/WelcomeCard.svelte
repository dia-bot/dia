<script lang="ts">
	// A pixel-faithful recreation of Dia's server-rendered welcome card, drawn in
	// pure CSS so it can re-render live as the customizer changes. Text scales
	// with the card width via container units, so it stays sharp at any size.
	let {
		from = '#FF6363',
		to = '#B244FC',
		angle = 45,
		color = '',
		image = '',
		title = 'Welcome, {user}!',
		subtitle = "You're member #{count} of {server}",
		footer = '',
		accent = '#FFFFFF',
		text = '#FFFFFF',
		subtext = '#F7E9F2',
		username = 'NewMember',
		count = 1024,
		server = 'Aurora',
		avatarText = ''
	}: {
		from?: string;
		to?: string;
		angle?: number;
		color?: string;
		image?: string;
		title?: string;
		subtitle?: string;
		footer?: string;
		accent?: string;
		text?: string;
		subtext?: string;
		username?: string;
		count?: number;
		server?: string;
		avatarText?: string;
	} = $props();

	const sub = (s: string) =>
		(s ?? '')
			.replaceAll('{user}', username)
			.replaceAll('{username}', username)
			.replaceAll('{count}', count.toLocaleString('en-US'))
			.replaceAll('{server}', server);

	const bg = $derived(
		image
			? `center / cover no-repeat url(${image})`
			: from && to
				? `linear-gradient(${angle}deg, ${from}, ${to})`
				: color || '#1F1B2E'
	);
	const ini = $derived(
		avatarText || username.replace(/[^a-zA-Z0-9]/g, '').slice(0, 2).toUpperCase() || '?'
	);
</script>

<div class="wc" style="background: {bg};">
	{#if image}
		<div class="wc-scrim"></div>
	{/if}
	<div class="wc-inner">
		<div class="wc-avatar" style="border-color: {accent};">
			<span style="color: {text};">{ini}</span>
		</div>
		<div class="wc-title" style="color: {text};">{sub(title)}</div>
		<div class="wc-sub" style="color: {subtext};">{sub(subtitle)}</div>
		{#if footer}
			<div class="wc-footer" style="color: {subtext};">{sub(footer)}</div>
		{/if}
		<div class="wc-accent" style="background: {accent};"></div>
	</div>
	<div class="wc-mark" style="color: {subtext};">dia</div>
</div>

<style>
	.wc {
		container-type: inline-size;
		position: relative;
		width: 100%;
		aspect-ratio: 1024 / 450;
		border-radius: 16px;
		overflow: hidden;
		isolation: isolate;
	}
	.wc-scrim {
		position: absolute;
		inset: 0;
		background: linear-gradient(180deg, rgba(20, 16, 30, 0.15), rgba(20, 16, 30, 0.55));
	}
	.wc-inner {
		position: absolute;
		inset: 0;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		text-align: center;
		padding: 6cqw;
	}
	.wc-avatar {
		width: clamp(56px, 19cqw, 132px);
		height: clamp(56px, 19cqw, 132px);
		border-radius: 999px;
		border: 0.9cqw solid #fff;
		background: rgba(255, 255, 255, 0.16);
		backdrop-filter: blur(2px);
		display: grid;
		place-items: center;
		margin-bottom: 3.4cqw;
		box-shadow: 0 6px 20px rgba(0, 0, 0, 0.18);
	}
	.wc-avatar span {
		font-size: clamp(18px, 7cqw, 48px);
		font-weight: 800;
		letter-spacing: -0.01em;
	}
	.wc-title {
		font-size: clamp(20px, 6.4cqw, 52px);
		font-weight: 800;
		line-height: 1.04;
		letter-spacing: -0.02em;
		text-shadow: 0 2px 14px rgba(0, 0, 0, 0.14);
	}
	.wc-sub {
		margin-top: 1.6cqw;
		font-size: clamp(11px, 2.9cqw, 22px);
		font-weight: 600;
		text-shadow: 0 1px 3px rgba(0, 0, 0, 0.32), 0 1px 12px rgba(0, 0, 0, 0.22);
	}
	.wc-footer {
		margin-top: 1cqw;
		font-size: clamp(10px, 2.2cqw, 16px);
		opacity: 0.85;
	}
	.wc-accent {
		margin-top: 3.2cqw;
		width: clamp(34px, 11cqw, 84px);
		height: 0.9cqw;
		min-height: 3px;
		border-radius: 999px;
		opacity: 0.92;
	}
	.wc-mark {
		position: absolute;
		right: 4cqw;
		bottom: 3cqw;
		font-size: clamp(9px, 2cqw, 15px);
		font-weight: 800;
		letter-spacing: 0.02em;
		opacity: 0.4;
	}
</style>
