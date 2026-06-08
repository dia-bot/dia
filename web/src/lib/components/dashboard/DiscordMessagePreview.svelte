<script lang="ts">
	// Renders a message exactly as Discord shows it — authentic Discord colours,
	// real mention pills, multiple embeds. Self-contained: it owns the sample-data
	// substitution so {user.mention} can render as a pill in rich text but as plain
	// "@Ada" in title/footer (matching how Discord treats each).
	import Logo from '$lib/components/Logo.svelte';

	type Field = { name: string; value: string; inline: boolean };
	type Embed = {
		enabled: boolean;
		color: string;
		author_name: string;
		author_icon: string;
		title: string;
		url: string;
		description: string;
		fields: Field[];
		thumbnail: string;
		image_url: string;
		footer_text: string;
		footer_icon: string;
		timestamp: boolean;
	};

	let {
		botName = 'Dia',
		content,
		embeds,
		cardEnabled,
		cardUrl = '',
		serverName = 'the server',
		serverId = ''
	}: {
		botName?: string;
		content: string;
		embeds: Embed[];
		cardEnabled: boolean;
		cardUrl?: string;
		serverName?: string;
		serverId?: string;
	} = $props();

	const NAME = 'Ada';
	const MENTION = '\uE000'; // private-use sentinel, swapped for a pill after escaping

	function sub(s: string): string {
		return (s ?? '')
			.replaceAll('{user.mention}', MENTION)
			.replaceAll('{user.name}', 'ada')
			.replaceAll('{username}', 'ada')
			.replaceAll('{user.id}', '123456789012345678')
			.replaceAll('{user.avatar}', '')
			.replaceAll('{user}', NAME)
			.replaceAll('{server.id}', serverId)
			.replaceAll('{server}', serverName || 'the server')
			.replaceAll('{count.ordinal}', '1,024th')
			.replaceAll('{count}', '1024');
	}
	function esc(s: string) {
		return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
	}
	// rich text: markdown + mention pills
	function md(s: string) {
		return esc(sub(s))
			.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
			.replace(/\*(.+?)\*/g, '<em>$1</em>')
			.replace(/`(.+?)`/g, '<code class="dc-code">$1</code>')
			.replace(/\n/g, '<br>')
			.replaceAll(MENTION, `<span class="dc-mention">@${NAME}</span>`);
	}
	// plain text fields (title, author, footer, field name): mention shows as @Ada
	function plain(s: string) {
		return sub(s).replaceAll(MENTION, `@${NAME}`);
	}

	const shown = $derived(embeds.filter((e) => e.enabled));
	const embedImage = (e: Embed) => (e.image_url.trim() === '{card}' ? cardUrl : plain(e.image_url));
	const cardLoose = $derived(
		cardEnabled && cardUrl && !shown.some((e) => e.image_url.trim() === '{card}')
	);
	const nothing = $derived(!content.trim() && shown.length === 0 && !cardLoose);
</script>

<div class="dc-root">
	<div class="dc-avatar"><Logo size={26} /></div>
	<div class="min-w-0 flex-1">
		<div class="dc-head">
			<span class="dc-name">{botName}</span>
			<span class="dc-badge">APP</span>
			<span class="dc-time">Today at 12:00</span>
		</div>

		{#if content.trim()}
			<div class="dc-content">{@html md(content)}</div>
		{/if}

		{#each shown as e, i (i)}
			<div class="dc-embed" style="border-left-color:{e.color || '#5865f2'}">
				<div class="min-w-0 flex-1">
					{#if e.author_name}
						<div class="dc-author">
							{#if plain(e.author_icon)}<img class="dc-author-ic" src={plain(e.author_icon)} alt="" />{/if}
							<span>{plain(e.author_name)}</span>
						</div>
					{/if}
					{#if e.title}<div class="dc-title" class:dc-link={!!e.url}>{plain(e.title)}</div>{/if}
					{#if e.description}<div class="dc-desc">{@html md(e.description)}</div>{/if}
					{#if e.fields.some((f) => f.name || f.value)}
						<div class="dc-fields">
							{#each e.fields.filter((f) => f.name || f.value) as f, fi (fi)}
								<div style="grid-column:{f.inline ? 'span 4' : '1 / -1'}">
									<div class="dc-field-name">{plain(f.name)}</div>
									<div class="dc-field-val">{@html md(f.value)}</div>
								</div>
							{/each}
						</div>
					{/if}
					{#if embedImage(e)}<img class="dc-image" src={embedImage(e)} alt="" />{/if}
					{#if e.footer_text || e.timestamp}
						<div class="dc-footer">
							{#if plain(e.footer_icon)}<img class="dc-footer-ic" src={plain(e.footer_icon)} alt="" />{/if}
							{#if e.footer_text}<span>{plain(e.footer_text)}</span>{/if}
							{#if e.footer_text && e.timestamp}<span>•</span>{/if}
							{#if e.timestamp}<span>Today at 12:00</span>{/if}
						</div>
					{/if}
				</div>
				{#if plain(e.thumbnail)}<img class="dc-thumb" src={plain(e.thumbnail)} alt="" />{/if}
			</div>
		{/each}

		{#if cardLoose}<img class="dc-image dc-image-loose" src={cardUrl} alt="" />{/if}

		{#if nothing}
			<div class="dc-empty">Nothing to send yet — add a message, an embed, or a card.</div>
		{/if}
	</div>
</div>

<style>
	.dc-root {
		display: flex;
		gap: 0.8rem;
		padding: 1rem 1.1rem;
		background: #313338;
		font-family: 'gg sans', var(--font-sans);
		color: #dbdee1;
		font-size: 0.95rem;
		line-height: 1.4;
	}
	.dc-avatar {
		display: grid;
		place-items: center;
		height: 2.5rem;
		width: 2.5rem;
		flex: none;
		border-radius: 50%;
		background: #1e1f22;
	}
	.dc-head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	.dc-name {
		font-weight: 600;
		color: #f2f3f5;
	}
	.dc-badge {
		background: #5865f2;
		color: #fff;
		font-size: 0.62rem;
		font-weight: 600;
		padding: 0.05rem 0.28rem;
		border-radius: 4px;
	}
	.dc-time {
		color: #949ba4;
		font-size: 0.72rem;
	}
	.dc-content {
		margin-top: 0.15rem;
		white-space: pre-wrap;
		word-break: break-word;
	}
	.dc-content :global(strong) {
		font-weight: 700;
	}
	.dc-content :global(.dc-mention),
	.dc-desc :global(.dc-mention) {
		background: rgba(88, 101, 242, 0.3);
		color: #c9cdfb;
		border-radius: 3px;
		padding: 0 2px;
		font-weight: 500;
	}
	.dc-content :global(.dc-code),
	.dc-desc :global(.dc-code) {
		background: #1e1f22;
		border-radius: 4px;
		padding: 0.05rem 0.3rem;
		font-family: var(--font-mono);
		font-size: 0.82em;
	}
	.dc-embed {
		display: flex;
		gap: 0.75rem;
		margin-top: 0.4rem;
		max-width: 520px;
		background: #2b2d31;
		border-left: 4px solid;
		border-radius: 4px;
		padding: 0.65rem 0.85rem 0.75rem;
	}
	.dc-author {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		font-size: 0.82rem;
		font-weight: 600;
		color: #f2f3f5;
		margin-bottom: 0.25rem;
	}
	.dc-author-ic {
		height: 1.1rem;
		width: 1.1rem;
		border-radius: 50%;
	}
	.dc-title {
		font-weight: 600;
		color: #f2f3f5;
		margin-bottom: 0.2rem;
	}
	.dc-link {
		color: #00a8fc;
	}
	.dc-desc {
		font-size: 0.88rem;
		white-space: pre-wrap;
		word-break: break-word;
	}
	.dc-fields {
		display: grid;
		grid-template-columns: repeat(12, 1fr);
		gap: 0.5rem;
		margin-top: 0.5rem;
	}
	.dc-field-name {
		font-size: 0.78rem;
		font-weight: 600;
		color: #f2f3f5;
	}
	.dc-field-val {
		font-size: 0.82rem;
		white-space: pre-wrap;
	}
	.dc-image {
		margin-top: 0.5rem;
		max-width: 100%;
		border-radius: 6px;
		display: block;
	}
	.dc-image-loose {
		max-width: 420px;
	}
	.dc-thumb {
		height: 5rem;
		width: 5rem;
		flex: none;
		border-radius: 6px;
		object-fit: cover;
	}
	.dc-footer {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		margin-top: 0.5rem;
		font-size: 0.72rem;
		color: #949ba4;
	}
	.dc-footer-ic {
		height: 1rem;
		width: 1rem;
		border-radius: 50%;
	}
	.dc-empty {
		margin-top: 0.3rem;
		font-size: 0.85rem;
		color: #949ba4;
		font-style: italic;
	}
</style>
