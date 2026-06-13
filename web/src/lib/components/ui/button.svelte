<script lang="ts" module>
	import { tv, type VariantProps } from 'tailwind-variants';

	export const buttonVariants = tv({
		base: 'inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-md text-[12.5px] font-medium ring-offset-background transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-3.5 [&_svg]:shrink-0',
		variants: {
			variant: {
				default: 'bg-primary text-primary-foreground hover:bg-primary/90',
				destructive: 'bg-destructive text-destructive-foreground hover:bg-destructive/90',
				outline:
					'border border-border bg-background text-foreground hover:bg-secondary hover:text-foreground',
				secondary: 'bg-secondary text-secondary-foreground hover:bg-secondary/80',
				ghost: 'text-muted-foreground hover:bg-secondary hover:text-foreground',
				link: 'text-foreground underline-offset-4 hover:underline'
			},
			size: {
				default: 'h-7 px-2.5',
				sm: 'h-6 px-2 text-[11.5px]',
				lg: 'h-8 px-3 text-[13px]',
				icon: 'h-7 w-7'
			}
		},
		defaultVariants: { variant: 'default', size: 'default' }
	});

	export type ButtonVariants = VariantProps<typeof buttonVariants>;
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLButtonAttributes, HTMLAnchorAttributes } from 'svelte/elements';
	import { cn } from '$lib/utils';

	type Props = (HTMLButtonAttributes | HTMLAnchorAttributes) & {
		variant?: ButtonVariants['variant'];
		size?: ButtonVariants['size'];
		href?: string;
		children?: Snippet;
		class?: string;
	};

	let {
		variant = 'default',
		size = 'default',
		href,
		class: cls,
		children,
		...rest
	}: Props = $props();
</script>

{#if href}
	<a {href} class={cn(buttonVariants({ variant, size }), cls)} {...rest as HTMLAnchorAttributes}>
		{#if children}{@render children()}{/if}
	</a>
{:else}
	<button class={cn(buttonVariants({ variant, size }), cls)} {...rest as HTMLButtonAttributes}>
		{#if children}{@render children()}{/if}
	</button>
{/if}
