// Starter card layouts — ready-made designs so anyone can pick one and tweak it
// in Card Studio instead of starting from a blank canvas. Each is a full Layout
// with {variable} bindings, so it works for any member out of the box.
import type { Layout } from './schema';

// The avatar is just a circular image bound to {{.User.Avatar}} — rounded by the corner
// radius and bordered with an outside stroke (no special mask/ring fields).
function avatar(x: number, y: number, size: number, ring: string, rw = 6): Layout['layers'][number] {
	return { id: 'avatar', type: 'image', name: 'Avatar', x, y, w: size, h: size, opacity: 1, hidden: false, src: '{{.User.Avatar}}', fit: 'cover', radius: 9999, stroke_color: ring, stroke_width: rw, stroke_align: 'outside' };
}
function text(id: string, name: string, x: number, y: number, w: number, h: number, t: string, size: number, weight: number, color: string, align: 'left' | 'center' | 'right'): Layout['layers'][number] {
	return { id, type: 'text', name, x, y, w, h, opacity: 1, hidden: false, text: t, font_size: size, font_weight: weight, color, align };
}

export interface CardTemplate {
	id: string;
	name: string;
	layout: Layout;
}

export const cardTemplates: CardTemplate[] = [
	{
		id: 'aurora',
		name: 'Aurora',
		layout: {
			width: 1024,
			height: 450,
			background: { type: 'gradient', from: '#FF6363', to: '#B244FC', angle: 45 },
			layers: [
				avatar(422, 48, 180, '#FFFFFF'),
				text('title', 'Title', 162, 252, 700, 64, 'Welcome, {{.User.Name}}!', 52, 700, '#FFFFFF', 'center'),
				text('subtitle', 'Subtitle', 162, 322, 700, 40, "You're member #{{.Count}} of {{.Server.Name}}", 26, 400, '#F1DFDF', 'center')
			]
		}
	},
	{
		id: 'midnight',
		name: 'Midnight',
		layout: {
			width: 1024,
			height: 450,
			background: { type: 'gradient', from: '#16131F', to: '#3A2E5C', angle: 30 },
			layers: [
				avatar(422, 50, 176, '#B244FC'),
				text('title', 'Title', 162, 252, 700, 64, 'Welcome, {{.User.Name}}', 50, 700, '#FFFFFF', 'center'),
				text('subtitle', 'Subtitle', 162, 322, 700, 40, '{{.Server.Name}} · {{.CountOrdinal}} member', 26, 400, '#C9C3DA', 'center')
			]
		}
	},
	{
		id: 'minimal',
		name: 'Minimal',
		layout: {
			width: 1024,
			height: 450,
			background: { type: 'solid', color: '#0E0E11' },
			layers: [
				avatar(80, 135, 180, '#222228', 4),
				text('title', 'Title', 300, 168, 640, 56, 'Welcome, {{.User.Name}}', 46, 700, '#FAFAFA', 'left'),
				text('subtitle', 'Subtitle', 300, 232, 640, 36, "You're our {{.CountOrdinal}} member", 24, 400, '#A4A4AE', 'left')
			]
		}
	},
	{
		id: 'spotlight',
		name: 'Spotlight',
		layout: {
			width: 1024,
			height: 450,
			background: { type: 'gradient', from: '#0A0A0C', to: '#1B1B22', angle: 0 },
			layers: [
				{ id: 'glow', type: 'rect', name: 'Glow', x: 332, y: 18, w: 360, h: 360, opacity: 0.18, hidden: false, fill: '#B244FC', radius: 360 },
				avatar(382, 40, 260, '#FFFFFF', 8),
				text('title', 'Title', 112, 320, 800, 64, '{{.User.Name}} just joined', 48, 700, '#FFFFFF', 'center'),
				text('subtitle', 'Subtitle', 112, 388, 800, 36, 'Member #{{.Count}} of {{.Server.Name}}', 24, 400, '#9AA0AA', 'center')
			]
		}
	}
];

// rankStarterLayout is the default rank card AND the seed the studio opens with
// when a guild first designs one. It is the FLAT house card: a solid charcoal
// canvas (no gradient) filling the full 934×282 space, an avatar on the left,
// the member's name + level/rank, and a rose XP bar over a hairline track. The
// palette matches the flat rank-card constants (bg #141417, text #FAFAFA, sub
// #A4A4AE, bar #FF6363, bar track #212126) and mirrors the Go leveling.Default()
// layout bit-for-bit so the dashboard preview and the bot's /rank agree.
export function rankStarterLayout(): Layout {
	return cloneLayout({
		width: 934,
		height: 282,
		background: { type: 'solid', color: '#141417' },
		layers: [
			// Avatar on the left, circular, with a subtle hairline ring.
			avatar(48, 51, 180, '#212126', 4),
			// Member name, large and bright.
			text('name', 'Username', 268, 52, 618, 62, '{{.User.Name}}', 48, 700, '#FAFAFA', 'left'),
			// Level / rank line, dimmed sub-text.
			text('meta', 'Level / Rank', 268, 122, 618, 36, 'LEVEL {{.Level}}    ·    RANK #{{.Rank}}', 26, 700, '#A4A4AE', 'left'),
			// XP progress-bar track (full width of the text column).
			{ id: 'bar-bg', type: 'rect', name: 'XP track', x: 268, y: 178, w: 618, h: 22, opacity: 1, hidden: false, fill: '#212126', radius: 11 },
			// XP progress fill: a rose bar bound to the member's XP progress. It spans
			// the full track (same w as bar-bg); `progress: true` makes the renderer
			// paint it to {{.Progress}} percent on the live card, so at 45% it fills
			// ~278px. Editing the width here only changes the empty-track look.
			{ id: 'bar', type: 'rect', name: 'XP fill', x: 268, y: 178, w: 618, h: 22, opacity: 1, hidden: false, fill: '#FF6363', radius: 11, progress: true },
			// XP figures under the bar.
			text('xp', 'XP', 268, 214, 618, 30, '{{.LevelXP}} / {{.LevelNeeded}} XP   ·   {{.Progress}}', 22, 400, '#A4A4AE', 'left')
		]
	});
}

// cloneLayout returns a deep, plain-object copy so editing never mutates a
// template (or a reactive proxy).
export function cloneLayout(l: Layout): Layout {
	return JSON.parse(JSON.stringify(l));
}

export function templateLayout(id: string): Layout {
	const t = cardTemplates.find((x) => x.id === id) ?? cardTemplates[0];
	return cloneLayout(t.layout);
}
