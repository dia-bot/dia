// Chip colour classes keyed by an action's tone. The interface is near
// monochrome (one rose accent only): neutral and warn stay paper/ink on
// hairlines, and only danger carries the rose. No other hues.
export const TONE_CHIP: Record<'neutral' | 'warn' | 'danger', string> = {
	neutral: 'border-line bg-ink-2 text-muted',
	warn: 'border-line-strong bg-surface text-ink',
	danger: 'border-accent/30 bg-blush text-accent-ink'
};
