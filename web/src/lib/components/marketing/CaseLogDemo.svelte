<script lang="ts">
	// The moderation case log — the durable record behind every /ban /kick
	// /timeout /warn and AutoMod action, mirroring the dashboard's case view.
	const cases = [
		{ n: 145, action: 'timeout', user: 'raid_alt', mod: 'AutoMod', reason: 'AutoMod · mention spam', active: true },
		{ n: 144, action: 'warn', user: 'rudeguy', mod: 'maya', reason: 'Banned word', active: false },
		{ n: 143, action: 'ban', user: 'scammer', mod: 'alex', reason: 'Phishing links', active: true },
		{ n: 142, action: 'kick', user: 'troll22', mod: 'maya', reason: 'Repeated spam after warning', active: false },
		{ n: 141, action: 'timeout', user: 'lurker', mod: 'AutoMod', reason: 'AutoMod · Discord invite', active: false }
	];

	function chip(action: string): string {
		switch (action) {
			case 'ban':
			case 'unban':
				return 'border-[color-mix(in_srgb,var(--color-danger)_35%,transparent)] bg-[color-mix(in_srgb,var(--color-danger)_18%,transparent)] text-[var(--color-danger)]';
			case 'kick':
				return 'border-line-strong bg-[color-mix(in_srgb,var(--color-pink)_16%,transparent)] text-pink';
			case 'timeout':
			case 'mute':
				return 'border-line-strong bg-blush text-accent-ink';
			case 'warn':
				return 'border-line-strong bg-[color-mix(in_srgb,#f0c674_16%,transparent)] text-[#f0c674]';
			default:
				return 'border-line-strong bg-ink-2 text-muted';
		}
	}
</script>

<div class="card overflow-hidden">
	<div class="flex items-center justify-between border-b border-line px-5 py-3.5">
		<h3 class="text-[15px] font-semibold">Recent cases</h3>
		<span class="font-mono text-xs text-faint">145 total</span>
	</div>
	<div class="overflow-x-auto">
		<table class="w-full text-sm">
			<thead>
				<tr class="border-b border-line bg-ink-2 text-left text-xs uppercase tracking-wide text-muted">
					<th class="px-4 py-2 font-medium">Case</th>
					<th class="px-4 py-2 font-medium">Action</th>
					<th class="px-4 py-2 font-medium">Member</th>
					<th class="px-4 py-2 font-medium">Moderator</th>
					<th class="px-4 py-2 font-medium">Reason</th>
				</tr>
			</thead>
			<tbody>
				{#each cases as c (c.n)}
					<tr class="border-b border-line last:border-b-0">
						<td class="px-4 py-3 font-medium tabular-nums text-muted">#{c.n}</td>
						<td class="px-4 py-3">
							<span
								class="inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-xs font-medium capitalize {chip(
									c.action
								)}"
							>
								{c.action}
								{#if c.active}<span class="inline-block h-1.5 w-1.5 rounded-full bg-current opacity-70"
									></span>{/if}
							</span>
						</td>
						<td class="px-4 py-3"><code class="text-accent-ink">@{c.user}</code></td>
						<td class="px-4 py-3"><code class="text-muted">@{c.mod}</code></td>
						<td class="px-4 py-3 text-ink">{c.reason}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
</div>
