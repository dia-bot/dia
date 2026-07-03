// The card template variables the Card Studio offers as click-to-insert chips.
// These mirror the nested data root the Go card renderer addresses (see
// internal/templating/card.go: DataFromVars), so every chip inserts a real Go
// text/template path — never a legacy {brace} token. `context` scopes a chip:
// 'all' shows on every card (welcome + rank); 'rank' only on rank cards, where
// the level/rank/XP/progress fields are populated.
//
// Keep in lockstep with internal/templating/card.go — the Go agent cross-checks
// these paths.
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
