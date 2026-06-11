// Barrel exports for the shadcn-svelte-style content primitives. Atomic
// components are default-exported from their own file; compound primitives
// (Dialog, Tabs, Popover, etc.) are grouped namespaces.

export { default as Button, buttonVariants } from './button.svelte';
export { default as Card } from './card.svelte';
export { default as CardHeader } from './card-header.svelte';
export { default as CardTitle } from './card-title.svelte';
export { default as CardDescription } from './card-description.svelte';
export { default as CardContent } from './card-content.svelte';
export { default as CardFooter } from './card-footer.svelte';
export { default as Input } from './input.svelte';
export { default as Textarea } from './textarea.svelte';
export { default as Label } from './label.svelte';
export { default as Badge, badgeVariants } from './badge.svelte';
export { default as Separator } from './separator.svelte';
export { default as Skeleton } from './skeleton.svelte';
export { default as Switch } from './switch.svelte';

export * as Tabs from './tabs/index.js';
export * as Popover from './popover/index.js';
export * as Dialog from './dialog/index.js';
export * as Command from './command/index.js';
export * as DropdownMenu from './dropdown-menu/index.js';
export * as Tooltip from './tooltip/index.js';
export * as Select from './select/index.js';
