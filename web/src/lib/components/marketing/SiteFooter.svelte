<script lang="ts">
	import Logo from '$lib/components/Logo.svelte';
	import Github from 'lucide-svelte/icons/github';

	// Company footer with the registered-entity disclosure. Shared across the site.
	const groups = [
		{
			title: 'Product',
			links: [
				{ label: 'Welcome', href: '/features/welcome' },
				{ label: 'Leveling', href: '/features/leveling' },
				{ label: 'Roles', href: '/features/roles' },
				{ label: 'Moderation', href: '/features/moderation' },
				{ label: 'Custom commands', href: '/features/commands' },
				{ label: 'Dashboard', href: '/features/dashboard' }
			]
		},
		{
			title: 'Project',
			links: [
				{ label: 'GitHub', href: 'https://github.com/dia-bot/dia' },
				{ label: 'Documentation', href: 'https://github.com/dia-bot/dia#readme' },
				{ label: 'Self-hosting', href: 'https://github.com/dia-bot/dia/tree/main/deploy' },
				{ label: 'Report an issue', href: 'https://github.com/dia-bot/dia/issues' }
			]
		},
		{
			title: 'Legal',
			links: [
				{ label: 'Terms of Service', href: '/terms' },
				{ label: 'Privacy Policy', href: '/privacy' },
				{ label: 'Contact', href: 'mailto:hello@dia.xyz' }
			]
		}
	];

	const year = 2026;
</script>

<footer class="border-t border-line bg-surface">
	<div class="mx-auto max-w-page px-6 py-14">
		<div class="grid gap-10 md:grid-cols-12">
			<div class="md:col-span-4">
				<Logo size={28} wordmark />
				<p class="mt-4 max-w-xs text-sm leading-relaxed text-muted">
					The open-source Discord bot with a realtime dashboard — welcome images, leveling,
					moderation, roles and custom commands, all in one.
				</p>
				<div class="mt-5 flex items-center gap-2">
					<span
						class="inline-flex items-center gap-1.5 rounded-full border border-line bg-bg px-2 py-0.5 text-[11px] font-medium text-muted"
					>
						<span class="h-1.5 w-1.5 rounded-full bg-success"></span> All systems operational
					</span>
				</div>
				<p class="mt-5 text-xs leading-relaxed text-muted">
					Mindroot&nbsp;Ltd · Company&nbsp;no.&nbsp;16543299<br />
					71–75 Shelton Street, London, WC2H&nbsp;9JQ<br />
					United Kingdom
				</p>
			</div>

			<div class="grid grid-cols-2 gap-8 sm:grid-cols-3 md:col-span-8">
				{#each groups as g (g.title)}
					<div>
						<h4 class="mb-3 text-[11px] font-semibold uppercase tracking-wider text-muted">
							{g.title}
						</h4>
						<ul class="space-y-2.5">
							{#each g.links as l (l.label)}
								<li>
									<a href={l.href} class="text-[13px] text-ink/75 transition-colors hover:text-accent-ink"
										>{l.label}</a
									>
								</li>
							{/each}
						</ul>
					</div>
				{/each}
			</div>
		</div>

		<div
			class="mt-12 flex flex-col items-start justify-between gap-3 border-t border-line pt-6 sm:flex-row sm:items-center"
		>
			<p class="text-xs text-muted">
				© {year} Dia. Mindroot Ltd · company no. 16543299 · 71–75 Shelton Street, London WC2H 9JQ.
			</p>
			<div class="flex items-center gap-4">
				<a
					href="https://github.com/dia-bot/dia"
					class="inline-flex items-center gap-1.5 text-xs text-muted transition-colors hover:text-ink"
				>
					<Github size={14} /> github.com/dia-bot/dia
				</a>
				<span class="text-xs text-faint">·</span>
				<span class="text-xs text-muted">Open source · MIT</span>
			</div>
		</div>
	</div>
</footer>
