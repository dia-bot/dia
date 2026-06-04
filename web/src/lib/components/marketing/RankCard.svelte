<script lang="ts">
	import { onMount } from 'svelte';

	// CSS recreation of Dia's generated rank card. The XP bar animates from 0 to
	// its real fill the first time the card scrolls into view.
	let {
		from = '#1F1B2E',
		to = '#3A2E5C',
		angle = 30,
		color = '',
		accent = '#B244FC',
		text = '#FFFFFF',
		subtext = '#C9C3DA',
		barColor = '#B244FC',
		barTrack = '',
		username = 'Member',
		rank = 1,
		level = 12,
		levelXp = 450,
		neededXp = 1000,
		totalXp = 53200,
		avatarText = ''
	}: {
		from?: string;
		to?: string;
		angle?: number;
		color?: string;
		accent?: string;
		text?: string;
		subtext?: string;
		barColor?: string;
		barTrack?: string;
		username?: string;
		rank?: number;
		level?: number;
		levelXp?: number;
		neededXp?: number;
		totalXp?: number;
		avatarText?: string;
	} = $props();

	const bg = $derived(
		from && to ? `linear-gradient(${angle}deg, ${from}, ${to})` : color || '#1F1B2E'
	);
	const pct = $derived(neededXp > 0 ? Math.max(0, Math.min(100, (levelXp / neededXp) * 100)) : 0);
	const ini = $derived(
		avatarText || username.replace(/[^a-zA-Z0-9]/g, '').slice(0, 2).toUpperCase() || '?'
	);

	let el: HTMLElement;
	let filled = $state(false);

	onMount(() => {
		const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
		if (reduce || typeof IntersectionObserver === 'undefined') {
			filled = true;
			return;
		}
		const io = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) {
					filled = true;
					io.disconnect();
				}
			},
			{ threshold: 0.45 }
		);
		io.observe(el);
		return () => io.disconnect();
	});
</script>

<div bind:this={el} class="rc" style="background: {bg};">
	<div class="rc-avatar" style="border-color: {accent};">
		<span style="color: {text};">{ini}</span>
		<span class="rc-ring" style="background: {accent};"></span>
	</div>

	<div class="rc-body">
		<div class="rc-top">
			<div class="rc-name" style="color: {text};">{username}</div>
			<div class="rc-stats">
				<span style="color: {subtext};">RANK <b style="color: {text};">#{rank}</b></span>
				<span style="color: {subtext};">LEVEL <b style="color: {accent};">{level}</b></span>
			</div>
		</div>

		<div class="rc-bar" style="background: {barTrack || 'rgba(255,255,255,0.16)'};">
			<div
				class="rc-fill"
				style="width: {filled ? pct : 0}%; background: {barColor}; box-shadow: 0 0 12px {barColor}66;"
			></div>
		</div>

		<div class="rc-xp">
			<span style="color: {subtext};">{totalXp.toLocaleString('en-US')} XP total</span>
			<span style="color: {subtext};">
				<b style="color: {text};">{levelXp.toLocaleString('en-US')}</b> / {neededXp.toLocaleString(
					'en-US'
				)} XP
			</span>
		</div>
	</div>
</div>

<style>
	.rc {
		container-type: inline-size;
		display: flex;
		align-items: center;
		gap: 4cqw;
		width: 100%;
		aspect-ratio: 934 / 282;
		padding: 5cqw 6cqw;
		border-radius: 16px;
		overflow: hidden;
	}
	.rc-avatar {
		position: relative;
		flex-shrink: 0;
		width: clamp(48px, 17cqw, 116px);
		height: clamp(48px, 17cqw, 116px);
		border-radius: 999px;
		border: 0.8cqw solid #fff;
		background: rgba(255, 255, 255, 0.14);
		display: grid;
		place-items: center;
		box-shadow: 0 6px 18px rgba(0, 0, 0, 0.25);
	}
	.rc-avatar span:first-child {
		font-size: clamp(16px, 6.5cqw, 44px);
		font-weight: 800;
		letter-spacing: -0.01em;
	}
	.rc-ring {
		position: absolute;
		right: 4%;
		bottom: 4%;
		width: 18%;
		height: 18%;
		min-width: 8px;
		min-height: 8px;
		border-radius: 999px;
		border: 2px solid rgba(0, 0, 0, 0.25);
	}
	.rc-body {
		flex: 1;
		min-width: 0;
	}
	.rc-top {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 2cqw;
	}
	.rc-name {
		font-size: clamp(16px, 5cqw, 38px);
		font-weight: 800;
		letter-spacing: -0.02em;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.rc-stats {
		display: flex;
		gap: 2.4cqw;
		font-size: clamp(9px, 2.1cqw, 16px);
		font-weight: 600;
		letter-spacing: 0.04em;
		white-space: nowrap;
	}
	.rc-stats b {
		font-weight: 800;
	}
	.rc-bar {
		margin-top: 3cqw;
		height: clamp(8px, 2.6cqw, 18px);
		border-radius: 999px;
		overflow: hidden;
	}
	.rc-fill {
		height: 100%;
		border-radius: 999px;
		transition: width 1.1s cubic-bezier(0.16, 1, 0.3, 1);
	}
	.rc-xp {
		margin-top: 1.8cqw;
		display: flex;
		justify-content: space-between;
		font-size: clamp(9px, 2.2cqw, 16px);
		font-weight: 500;
	}
	.rc-xp b {
		font-weight: 700;
	}
</style>
