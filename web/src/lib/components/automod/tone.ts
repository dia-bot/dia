// Chip colour classes keyed by an action's tone. Neutral = paper chip, warn =
// amber, danger = rose. Kept tiny so cards and the editor share one source.
export const TONE_CHIP: Record<'neutral' | 'warn' | 'danger', string> = {
	neutral: 'border-line bg-ink-2 text-muted',
	warn: 'border-amber-500/30 bg-amber-500/10 text-amber-300',
	danger: 'border-accent/30 bg-blush text-accent-ink'
};
