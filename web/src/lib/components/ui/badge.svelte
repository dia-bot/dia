<script lang="ts" module>
	import { tv, type VariantProps } from 'tailwind-variants';

	export const badgeVariants = tv({
		base: 'inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-[10px] font-medium font-mono uppercase tracking-wider transition-colors focus:outline-none focus:ring-1 focus:ring-ring',
		variants: {
			variant: {
				default: 'border-transparent bg-primary text-primary-foreground',
				secondary: 'border-border bg-secondary text-secondary-foreground',
				outline: 'border-border text-muted-foreground',
				destructive: 'border-destructive/30 bg-destructive/10 text-destructive',
				success: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-400',
				warning: 'border-amber-500/30 bg-amber-500/10 text-amber-400'
			}
		},
		defaultVariants: { variant: 'default' }
	});

	export type BadgeVariants = VariantProps<typeof badgeVariants>;
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { cn } from '$lib/utils';

	type Props = HTMLAttributes<HTMLSpanElement> & {
		variant?: BadgeVariants['variant'];
		class?: string;
		children: Snippet;
	};

	let { variant = 'default', class: cls, children, ...rest }: Props = $props();
</script>

<span class={cn(badgeVariants({ variant }), cls)} {...rest}>
	{@render children()}
</span>
