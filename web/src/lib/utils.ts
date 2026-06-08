import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

// cn merges conditional class lists and resolves Tailwind conflicts — the
// standard shadcn helper, used by the Bits UI–based components in lib/components/ui.
export function cn(...inputs: ClassValue[]) {
	return twMerge(clsx(inputs));
}
