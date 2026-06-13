// Resolve a lucide icon component from the string names stored in the
// step-kind / option-kind catalogues (types.ts). Falls back to Square.
import * as icons from 'lucide-svelte';

export type LucideIcon = typeof icons.Square;

export function iconFor(name: string): LucideIcon {
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	return ((icons as any)[name] ?? icons.Square) as LucideIcon;
}
