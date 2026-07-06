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
	},
	{
		snippet: '{{ getKV "" }}',
		label: 'getKV',
		hint: "this member's stored value (set it with a shared kv_set in a command/automation)"
	},
	{
		snippet: '{{ getGuildKV "" }}',
		label: 'getGuildKV',
		hint: 'a guild-wide stored value (from a shared kv_set)'
	}
];

// A property that can be driven by a formula. `key` matches Layer.bind on the Go
// side; `kind` is the value the formula must produce; `types` scopes which layer
// types offer it (undefined = all); `group` buckets it in the Formulas editor;
// `values` lists the valid outputs for an enum (shown as hints). Keep in lockstep
// with the resolver keys in internal/imaging/layout.go resolveLayerBindings.
export type BindKind = 'number' | 'color' | 'bool' | 'enum' | 'string';
export type BindGroup =
	| 'Size & position'
	| 'Appearance'
	| 'Text'
	| 'Stroke'
	| 'Brush'
	| 'Mask'
	| 'Path';
export interface BindableProp {
	key: string;
	label: string;
	kind: BindKind;
	group: BindGroup;
	types?: LayerType[];
	values?: string[]; // enum: the valid outputs a formula must produce
}
const VECTOR: LayerType[] = ['rect', 'ellipse', 'path'];
export const BINDABLE_PROPS: BindableProp[] = [
	// Size & position — every layer.
	{ key: 'w', label: 'Width', kind: 'number', group: 'Size & position' },
	{ key: 'h', label: 'Height', kind: 'number', group: 'Size & position' },
	{ key: 'x', label: 'X', kind: 'number', group: 'Size & position' },
	{ key: 'y', label: 'Y', kind: 'number', group: 'Size & position' },
	{ key: 'rotation', label: 'Rotation°', kind: 'number', group: 'Size & position' },
	// Appearance.
	{ key: 'opacity', label: 'Opacity (0–1)', kind: 'number', group: 'Appearance' },
	{ key: 'hidden', label: 'Hidden', kind: 'bool', group: 'Appearance' },
	{ key: 'fill', label: 'Fill color', kind: 'color', group: 'Appearance', types: VECTOR },
	{ key: 'radius', label: 'Corner radius', kind: 'number', group: 'Appearance', types: ['rect', 'image'] },
	{ key: 'corner_tl', label: 'Corner ↖', kind: 'number', group: 'Appearance', types: ['rect', 'image'] },
	{ key: 'corner_tr', label: 'Corner ↗', kind: 'number', group: 'Appearance', types: ['rect', 'image'] },
	{ key: 'corner_br', label: 'Corner ↘', kind: 'number', group: 'Appearance', types: ['rect', 'image'] },
	{ key: 'corner_bl', label: 'Corner ↙', kind: 'number', group: 'Appearance', types: ['rect', 'image'] },
	{ key: 'fit', label: 'Image fit', kind: 'enum', group: 'Appearance', types: ['image'], values: ['cover', 'contain'] },
	{ key: 'progress', label: 'XP progress bar', kind: 'bool', group: 'Appearance', types: ['rect'] },
	// Text.
	{ key: 'color', label: 'Text color', kind: 'color', group: 'Text', types: ['text'] },
	{ key: 'font_size', label: 'Font size', kind: 'number', group: 'Text', types: ['text'] },
	{ key: 'font_weight', label: 'Font weight', kind: 'number', group: 'Text', types: ['text'] },
	{ key: 'letter_spacing', label: 'Letter spacing', kind: 'number', group: 'Text', types: ['text'] },
	{ key: 'line_height', label: 'Line height', kind: 'number', group: 'Text', types: ['text'] },
	{ key: 'align', label: 'Align', kind: 'enum', group: 'Text', types: ['text'], values: ['left', 'center', 'right'] },
	{ key: 'valign', label: 'Vertical align', kind: 'enum', group: 'Text', types: ['text'], values: ['top', 'middle', 'bottom'] },
	{ key: 'text_case', label: 'Case', kind: 'enum', group: 'Text', types: ['text'], values: ['none', 'upper', 'lower', 'title'] },
	{ key: 'text_decoration', label: 'Decoration', kind: 'enum', group: 'Text', types: ['text'], values: ['none', 'underline', 'strike'] },
	{ key: 'font_family', label: 'Font family', kind: 'string', group: 'Text', types: ['text'] },
	// Stroke.
	{ key: 'stroke_color', label: 'Stroke color', kind: 'color', group: 'Stroke', types: [...VECTOR, 'text'] },
	{ key: 'stroke_width', label: 'Stroke weight', kind: 'number', group: 'Stroke', types: [...VECTOR, 'text'] },
	{ key: 'stroke_align', label: 'Stroke position', kind: 'enum', group: 'Stroke', types: VECTOR, values: ['inside', 'center', 'outside'] },
	{ key: 'stroke_style', label: 'Stroke style', kind: 'enum', group: 'Stroke', types: VECTOR, values: ['solid', 'dashed'] },
	{ key: 'dash', label: 'Dash length', kind: 'number', group: 'Stroke', types: VECTOR },
	{ key: 'gap', label: 'Dash gap', kind: 'number', group: 'Stroke', types: VECTOR },
	{ key: 'stroke_cap', label: 'Stroke cap', kind: 'enum', group: 'Stroke', types: VECTOR, values: ['butt', 'round', 'square'] },
	{ key: 'stroke_join', label: 'Stroke join', kind: 'enum', group: 'Stroke', types: VECTOR, values: ['miter', 'bevel', 'round'] },
	{ key: 'miter_angle', label: 'Miter angle°', kind: 'number', group: 'Stroke', types: VECTOR },
	{ key: 'start_cap', label: 'Start arrow', kind: 'enum', group: 'Stroke', types: ['path'], values: ['none', 'line', 'arrow', 'triangle', 'circle', 'diamond'] },
	{ key: 'end_cap', label: 'End arrow', kind: 'enum', group: 'Stroke', types: ['path'], values: ['none', 'line', 'arrow', 'triangle', 'circle', 'diamond'] },
	// Brush — path-only advanced stroke.
	{ key: 'brush_name', label: 'Brush', kind: 'string', group: 'Brush', types: ['path'] },
	{ key: 'brush_direction', label: 'Brush direction', kind: 'enum', group: 'Brush', types: ['path'], values: ['forward', 'backward'] },
	{ key: 'width_profile', label: 'Width profile', kind: 'enum', group: 'Brush', types: ['path'], values: ['uniform', 'taper_start', 'taper_end', 'taper', 'lens'] },
	{ key: 'scatter_gap', label: 'Scatter gap', kind: 'number', group: 'Brush', types: ['path'] },
	{ key: 'scatter_wiggle', label: 'Scatter wiggle %', kind: 'number', group: 'Brush', types: ['path'] },
	{ key: 'scatter_size', label: 'Scatter size %', kind: 'number', group: 'Brush', types: ['path'] },
	{ key: 'scatter_rotation', label: 'Scatter rotation°', kind: 'number', group: 'Brush', types: ['path'] },
	{ key: 'scatter_angular', label: 'Scatter jitter°', kind: 'number', group: 'Brush', types: ['path'] },
	{ key: 'dynamic_frequency', label: 'Wobble frequency', kind: 'number', group: 'Brush', types: ['path'] },
	{ key: 'dynamic_wiggle', label: 'Wobble amount %', kind: 'number', group: 'Brush', types: ['path'] },
	{ key: 'dynamic_smoothen', label: 'Wobble smoothing', kind: 'number', group: 'Brush', types: ['path'] },
	// Mask — any layer can act as a stencil.
	{ key: 'clip', label: 'Use as mask', kind: 'bool', group: 'Mask' },
	{ key: 'clip_mode', label: 'Mask mode', kind: 'enum', group: 'Mask', values: ['alpha', 'vector', 'luminance'] },
	{ key: 'clip_invert', label: 'Invert mask', kind: 'bool', group: 'Mask' },
	// Path.
	{ key: 'closed', label: 'Closed path', kind: 'bool', group: 'Path', types: ['path'] }
];

// Order the groups appear in the Formulas editor.
export const BIND_GROUPS: BindGroup[] = [
	'Size & position',
	'Appearance',
	'Text',
	'Stroke',
	'Brush',
	'Mask',
	'Path'
];

// bindablePropsFor returns the properties offered for a given layer type.
export function bindablePropsFor(type: LayerType): BindableProp[] {
	return BINDABLE_PROPS.filter((p) => !p.types || p.types.includes(type));
}

// Formula-drivable CANVAS BACKGROUND properties (Background.bind on the Go side).
// A bound colour/gradient forces the legacy solid/gradient path server-side.
export const BG_BINDABLE_PROPS: BindableProp[] = [
	{ key: 'color', label: 'Background color', kind: 'color', group: 'Appearance' },
	{ key: 'from', label: 'Gradient from', kind: 'color', group: 'Appearance' },
	{ key: 'to', label: 'Gradient to', kind: 'color', group: 'Appearance' },
	{ key: 'angle', label: 'Gradient angle°', kind: 'number', group: 'Appearance' },
	{ key: 'blur', label: 'Blur', kind: 'number', group: 'Appearance' }
];

// Editable TEST values for the live server-render preview: overriding a {token}
// re-renders the card AS IF the member had that value, so formulas can be tested
// (set Level 60 to watch a level-gated colour flip). `token` is the flat {brace}
// key the render endpoint's ExtraVars expects (see internal/api/layout.go).
export interface TestVar {
	token: string;
	label: string;
	context: 'all' | 'rank';
	numeric?: boolean;
}
export const CARD_TEST_VARS: TestVar[] = [
	{ token: '{user}', label: 'Name', context: 'all' },
	{ token: '{count}', label: 'Member #', context: 'all', numeric: true },
	{ token: '{level}', label: 'Level', context: 'rank', numeric: true },
	{ token: '{rank}', label: 'Rank', context: 'rank', numeric: true },
	{ token: '{xp}', label: 'Total XP', context: 'rank', numeric: true },
	{ token: '{level.xp}', label: 'Level XP', context: 'rank', numeric: true },
	{ token: '{level.needed}', label: 'XP needed', context: 'rank', numeric: true },
	{ token: '{progress}', label: 'Progress', context: 'rank' }
];
export function cardTestVarsFor(ctx: 'welcome' | 'rank'): TestVar[] {
	return ctx === 'rank' ? CARD_TEST_VARS : CARD_TEST_VARS.filter((v) => v.context === 'all');
}
