// Starter card layouts — ready-made designs so anyone can pick one and tweak it
// in Card Studio instead of starting from a blank canvas. Each is a full Layout
// with {variable} bindings, so it works for any member out of the box.
import type { Layout } from './schema';

function avatar(x: number, y: number, size: number, ring: string, rw = 6): Layout['layers'][number] {
	return { id: 'avatar', type: 'avatar', name: 'Avatar', x, y, w: size, h: size, opacity: 1, hidden: false, src: '{{.User.Avatar}}', shape: 'circle', ring_color: ring, ring_width: rw, radius: 24 };
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

// rankStarterLayout is the default canvas when a guild first designs its rank
// card in Card Studio — sized for a rank card (934×282) with rank {tokens}.
export function rankStarterLayout(): Layout {
	return cloneLayout({
		width: 934,
		height: 282,
		background: { type: 'gradient', from: '#1F1B2E', to: '#3A2E5C', angle: 30 },
		layers: [
			avatar(48, 51, 180, '#B244FC', 6),
			text('name', 'Username', 260, 56, 630, 60, '{{.User.Name}}', 46, 700, '#FFFFFF', 'left'),
			text('meta', 'Level / Rank', 260, 124, 630, 36, 'Level {{.Level}}   ·   Rank #{{.Rank}}', 28, 400, '#C9C3DA', 'left'),
			text('xp', 'XP', 260, 178, 630, 32, '{{.LevelXP}} / {{.LevelNeeded}} XP   ({{.Progress}})', 24, 400, '#9AA0AA', 'left')
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
