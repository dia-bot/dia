// The card template variables the Card Studio offers as click-to-insert chips.
// These mirror the nested data root the Go card renderer addresses (see
// internal/templating/card.go: DataFromVars), so every chip inserts a real Go
// text/template path — never a legacy {brace} token. `context` scopes a chip:
// 'all' shows on every card (welcome + rank); 'rank' only on rank cards, where
// the level/rank/XP/progress fields are populated.
//
// Keep in lockstep with internal/templating/card.go — the Go agent cross-checks
// these paths.
import type { LayerType } from './schema';

export interface CardVar {
	tmpl: string; // the exact Go-template token inserted at the cursor
	label: string; // short chip label
	context: 'all' | 'rank';
}

export const CARD_VARS: CardVar[] = [
	// Member + server — available on every card.
	{ tmpl: '{{.User.Name}}', label: 'Name', context: 'all' },
	{ tmpl: '{{.User.Username}}', label: 'Username', context: 'all' },
	{ tmpl: '{{.User.Avatar}}', label: 'Avatar', context: 'all' },
	{ tmpl: '{{.Guild.Name}}', label: 'Server', context: 'all' },
	{ tmpl: '{{.Server.Name}}', label: 'Server (alt)', context: 'all' },
	{ tmpl: '{{.Guild.Icon}}', label: 'Server icon', context: 'all' },
	{ tmpl: '{{.Guild.MemberCount}}', label: 'Members', context: 'all' },
	{ tmpl: '{{.Count}}', label: 'Join #', context: 'all' },
	{ tmpl: '{{.CountOrdinal}}', label: 'Join # ordinal', context: 'all' },
	// Rank-only — level / rank / XP / progress are empty on welcome cards.
	{ tmpl: '{{.Level}}', label: 'Level', context: 'rank' },
	{ tmpl: '{{.Rank}}', label: 'Rank', context: 'rank' },
	{ tmpl: '{{.XP}}', label: 'Total XP', context: 'rank' },
	{ tmpl: '{{.LevelXP}}', label: 'Level XP', context: 'rank' },
	{ tmpl: '{{.LevelNeeded}}', label: 'XP needed', context: 'rank' },
	{ tmpl: '{{.Progress}}', label: 'Progress', context: 'rank' }
];

// cardVarsFor returns the chips for a studio context: a rank card shows every
// variable; a welcome card shows only the context-'all' set.
export function cardVarsFor(ctx: 'welcome' | 'rank'): CardVar[] {
	return ctx === 'rank' ? CARD_VARS : CARD_VARS.filter((v) => v.context === 'all');
}

// Numeric siblings of the display vars, for FORMULAS (layer property bindings).
// These are real numbers (not the formatted strings above), so math and
// comparisons work: `{{ fmul .ProgressFrac 618 }}`, `{{ if gt .LevelNum 50 }}`.
// Mirrors the numeric fields added in internal/templating/card.go DataFromVars.
export const CARD_NUM_VARS: CardVar[] = [
	{ tmpl: '{{.ProgressFrac}}', label: 'Progress 0–1', context: 'rank' },
	{ tmpl: '{{.ProgressPct}}', label: 'Progress 0–100', context: 'rank' },
	{ tmpl: '{{.LevelNum}}', label: 'Level #', context: 'rank' },
	{ tmpl: '{{.RankNum}}', label: 'Rank #', context: 'rank' },
	{ tmpl: '{{.XpNum}}', label: 'Total XP #', context: 'rank' },
	{ tmpl: '{{.LevelXpNum}}', label: 'Level XP #', context: 'rank' },
	{ tmpl: '{{.NeededNum}}', label: 'XP needed #', context: 'rank' },
	{ tmpl: '{{.MemberCount}}', label: 'Members #', context: 'all' }
];

// cardFormulaVarsFor returns the display + numeric vars offered in the Formulas
// picker for a studio context.
export function cardFormulaVarsFor(ctx: 'welcome' | 'rank'): CardVar[] {
	const all = [...CARD_VARS, ...CARD_NUM_VARS];
	return ctx === 'rank' ? all : all.filter((v) => v.context === 'all');
}

// Formula helper functions offered as click-to-insert snippets in the picker.
// `snippet` is inserted at the cursor. Mirrors the float-math funcs in
// internal/templating/card.go (cardMathFuncs) plus Go template built-ins.
export interface CardFunc {
	snippet: string;
	label: string;
	hint: string;
}
export const CARD_FUNCS: CardFunc[] = [
	{ snippet: '{{ fmul  1 }}', label: 'fmul', hint: 'multiply a × b, e.g. fmul .ProgressFrac 618' },
	{ snippet: '{{ fadd  1 }}', label: 'fadd', hint: 'add a + b' },
	{ snippet: '{{ fsub  1 }}', label: 'fsub', hint: 'subtract a − b' },
	{ snippet: '{{ fdiv  1 }}', label: 'fdiv', hint: 'divide a ÷ b' },
	{ snippet: '{{ round  }}', label: 'round', hint: 'round to a whole number' },
	{ snippet: '{{ clamp  0 100 }}', label: 'clamp', hint: 'clamp value between lo and hi' },
	{ snippet: '{{ lerp 0 100  }}', label: 'lerp', hint: 'blend a→b by t (0..1)' },
	{ snippet: '{{ fmin  0 }}', label: 'min', hint: 'smaller of two' },
	{ snippet: '{{ fmax  0 }}', label: 'max', hint: 'larger of two' },
	{
		snippet: '{{ if gt .LevelNum 50 }}A{{ else }}B{{ end }}',
		label: 'if / else',
		hint: 'pick A or B by a condition (gt, lt, eq, ge, le)'
	}
];

// A property that can be driven by a formula. `key` matches Layer.bind on the Go
// side; `kind` is the value the formula must produce; `types` scopes which layer
// types offer it (undefined = all). Drives the Formulas inspector section.
export interface BindableProp {
	key: string;
	label: string;
	kind: 'number' | 'color' | 'bool';
	types?: LayerType[];
}
export const BINDABLE_PROPS: BindableProp[] = [
	{ key: 'w', label: 'Width', kind: 'number' },
	{ key: 'h', label: 'Height', kind: 'number' },
	{ key: 'x', label: 'X', kind: 'number' },
	{ key: 'y', label: 'Y', kind: 'number' },
	{ key: 'opacity', label: 'Opacity (0–1)', kind: 'number' },
	{ key: 'rotation', label: 'Rotation°', kind: 'number' },
	{ key: 'hidden', label: 'Hidden (true/false)', kind: 'bool' },
	{ key: 'radius', label: 'Corner radius', kind: 'number', types: ['rect', 'image'] },
	{ key: 'font_size', label: 'Font size', kind: 'number', types: ['text'] },
	{ key: 'letter_spacing', label: 'Letter spacing', kind: 'number', types: ['text'] },
	{ key: 'line_height', label: 'Line height', kind: 'number', types: ['text'] },
	{ key: 'color', label: 'Text color', kind: 'color', types: ['text'] },
	{ key: 'fill', label: 'Fill color', kind: 'color', types: ['rect', 'ellipse', 'path'] },
	{
		key: 'stroke_color',
		label: 'Stroke color',
		kind: 'color',
		types: ['rect', 'ellipse', 'path', 'text']
	}
];

// bindablePropsFor returns the properties offered for a given layer type.
export function bindablePropsFor(type: LayerType): BindableProp[] {
	return BINDABLE_PROPS.filter((p) => !p.types || p.types.includes(type));
}
