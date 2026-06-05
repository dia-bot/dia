// Shared model for the element-based welcome-card editor. Positions are
// percentages of the canvas so a card renders identically at any size; font
// sizes are container-query units (cqw) for the same reason.
export type Anchor = 'center' | 'left' | 'right';
export type ElKind = 'text' | 'avatar' | 'badge' | 'divider' | 'rect' | 'bar';
export type Role = 'title' | 'subtitle' | 'accent' | 'custom';

export type El = {
	id: string;
	kind: ElKind;
	name: string;
	role: Role;
	x: number; // % of canvas width
	y: number; // % of canvas height
	anchor: Anchor;
	opacity: number; // 0..1
	rotation: number; // deg
	visible: boolean;
	locked: boolean;
	// text / badge
	text?: string;
	font?: number; // cqw
	weight?: number;
	color?: string;
	align?: Anchor;
	maxw?: number; // % width, for wrapping
	letter?: number; // em letter-spacing
	// badge
	bg?: string;
	// avatar
	size?: number; // % width (diameter)
	ring?: string;
	shape?: 'circle' | 'rounded' | 'square';
	// rect
	w?: number; // % width
	height?: number; // cqw
	radius?: number; // px
	// divider
	thickness?: number; // px
	// bar (rank XP progress)
	value?: number; // 0..100 fill
	track?: string; // bar track colour
};

export type Background = {
	type: 'gradient' | 'solid' | 'image';
	from: string;
	to: string;
	angle: number;
	color: string;
	image: string;
};

export type Theme = {
	id: string;
	name: string;
	bg: Background;
	text: string;
	subtext: string;
	accent: string;
};

export const THEMES: Theme[] = [
	{
		id: 'aurora',
		name: 'Aurora',
		bg: { type: 'gradient', from: '#FF6363', to: '#B244FC', angle: 45, color: '', image: '' },
		text: '#FFFFFF',
		subtext: '#F7E9F2',
		accent: '#FFFFFF'
	},
	{
		id: 'midnight',
		name: 'Midnight',
		bg: { type: 'gradient', from: '#1F1B2E', to: '#3A2E5C', angle: 30, color: '', image: '' },
		text: '#FFFFFF',
		subtext: '#C9C3DA',
		accent: '#B244FC'
	},
	{
		id: 'mono',
		name: 'Mono',
		bg: { type: 'solid', from: '', to: '', angle: 0, color: '#18181B', image: '' },
		text: '#FFFFFF',
		subtext: '#A1A1AA',
		accent: '#B244FC'
	},
	{
		id: 'blush',
		name: 'Blush',
		bg: { type: 'solid', from: '', to: '', angle: 0, color: '#F1DFDF', image: '' },
		text: '#2B2233',
		subtext: '#7A6B73',
		accent: '#FF6363'
	},
	{
		id: 'sunset',
		name: 'Sunset',
		bg: { type: 'gradient', from: '#FF6363', to: '#FFB347', angle: 60, color: '', image: '' },
		text: '#FFFFFF',
		subtext: '#FFF1E6',
		accent: '#FFFFFF'
	},
	{
		id: 'ocean',
		name: 'Ocean',
		bg: { type: 'gradient', from: '#0EA5E9', to: '#2563EB', angle: 40, color: '', image: '' },
		text: '#FFFFFF',
		subtext: '#DBEAFE',
		accent: '#FFFFFF'
	}
];

let uid = 0;
export const nextId = () => `el${++uid}`;

// Layout templates produce a fresh element set positioned for a given theme.
export function templateElements(id: string, t: Theme, mode: 'welcome' | 'rank' = 'welcome'): El[] {
	const base = {
		opacity: 1,
		rotation: 0,
		visible: true,
		locked: false
	};
	const title = (over: Partial<El>): El => ({
		id: nextId(),
		kind: 'text',
		name: 'Title',
		role: 'title',
		x: 50,
		y: 50,
		anchor: 'center',
		text: 'Welcome, {user}!',
		font: 6.4,
		weight: 800,
		color: t.text,
		align: 'center',
		maxw: 86,
		letter: -0.02,
		...base,
		...over
	});
	const subtitle = (over: Partial<El>): El => ({
		id: nextId(),
		kind: 'text',
		name: 'Subtitle',
		role: 'subtitle',
		x: 50,
		y: 50,
		anchor: 'center',
		text: "You're member #{count} of {server}",
		font: 2.9,
		weight: 600,
		color: t.subtext,
		align: 'center',
		maxw: 84,
		letter: 0,
		...base,
		...over
	});
	const avatar = (over: Partial<El>): El => ({
		id: nextId(),
		kind: 'avatar',
		name: 'Avatar',
		role: 'custom',
		x: 50,
		y: 32,
		anchor: 'center',
		size: 19,
		ring: t.accent,
		shape: 'circle',
		color: t.text,
		...base,
		...over
	});
	const badge = (over: Partial<El>): El => ({
		id: nextId(),
		kind: 'badge',
		name: 'Badge',
		role: 'accent',
		x: 50,
		y: 82,
		anchor: 'center',
		text: '1,024th member',
		font: 2.1,
		weight: 700,
		color: t.id === 'blush' ? '#FFFFFF' : t.bg.color || t.bg.to || '#1F1B2E',
		bg: t.accent,
		...base,
		...over
	});
	const divider = (over: Partial<El>): El => ({
		id: nextId(),
		kind: 'divider',
		name: 'Divider',
		role: 'accent',
		x: 50,
		y: 50,
		anchor: 'center',
		w: 12,
		thickness: 3,
		color: t.accent,
		...base,
		...over
	});
	const rect = (over: Partial<El>): El => ({
		id: nextId(),
		kind: 'rect',
		name: 'Panel',
		role: 'accent',
		x: 50,
		y: 50,
		anchor: 'center',
		w: 40,
		height: 80,
		radius: 18,
		color: t.accent,
		opacity: 0.16,
		rotation: 0,
		visible: true,
		locked: false
	});
	const ctext = (over: Partial<El>): El => ({
		id: nextId(),
		kind: 'text',
		name: 'Text',
		role: 'subtitle',
		x: 50,
		y: 50,
		anchor: 'center',
		text: 'Text',
		font: 2.8,
		weight: 700,
		color: t.subtext,
		align: 'center',
		maxw: 70,
		letter: 0.02,
		...base,
		...over
	});
	const bar = (over: Partial<El>): El => ({
		id: nextId(),
		kind: 'bar',
		name: 'XP bar',
		role: 'accent',
		x: 50,
		y: 50,
		anchor: 'center',
		w: 60,
		height: 3.4,
		color: t.accent,
		track: 'rgba(255,255,255,0.18)',
		value: 62,
		...base,
		...over
	});

	if (mode === 'rank') {
		switch (id) {
			case 'centered':
				return [
					avatar({ x: 50, y: 30, size: 22 }),
					title({ x: 50, y: 58, font: 6, text: '{user}' }),
					ctext({ x: 50, y: 72, font: 2.6, text: 'RANK #{rank} · LEVEL {level}' }),
					bar({ x: 50, y: 84, w: 70 }),
					subtitle({ x: 50, y: 93, font: 2.3, text: '{xp} / {nextxp} XP' })
				];
			case 'minimal':
				return [
					title({ x: 7, y: 34, anchor: 'left', align: 'left', font: 6.5, text: '{user}', maxw: 70 }),
					ctext({ x: 7, y: 54, anchor: 'left', align: 'left', font: 2.8, text: 'LEVEL {level} · RANK #{rank}', maxw: 70 }),
					bar({ x: 7, y: 70, anchor: 'left', w: 86 }),
					subtitle({ x: 93, y: 84, anchor: 'right', align: 'right', font: 2.5, text: '{xp} / {nextxp} XP' })
				];
			case 'classic':
			default:
				return [
					avatar({ x: 10, y: 50, anchor: 'left', size: 30 }),
					title({ x: 29, y: 33, anchor: 'left', align: 'left', font: 6, text: '{user}', maxw: 48 }),
					ctext({ x: 95, y: 30, anchor: 'right', align: 'right', font: 2.8, text: 'RANK #{rank}', maxw: 42 }),
					ctext({ x: 95, y: 45, anchor: 'right', align: 'right', font: 3.2, text: 'LEVEL {level}', role: 'accent', color: t.accent, name: 'Level', maxw: 42 }),
					bar({ x: 29, y: 66, anchor: 'left', w: 66 }),
					subtitle({ x: 95, y: 82, anchor: 'right', align: 'right', font: 2.6, text: '{xp} / {nextxp} XP' })
				];
		}
	}

	switch (id) {
		case 'banner':
			return [
				avatar({ x: 7, y: 50, anchor: 'left', size: 30 }),
				title({ x: 45, y: 40, anchor: 'left', align: 'left', font: 5, maxw: 52 }),
				divider({ x: 45, y: 52, anchor: 'left', w: 9 }),
				subtitle({ x: 45, y: 62, anchor: 'left', align: 'left', font: 2.5, maxw: 52 })
			];
		case 'minimal':
			return [
				title({ x: 8, y: 40, anchor: 'left', align: 'left', font: 7, maxw: 80 }),
				subtitle({ x: 8, y: 62, anchor: 'left', align: 'left', font: 3, maxw: 70 })
			];
		case 'spotlight':
			return [
				avatar({ x: 50, y: 36, size: 22 }),
				title({ x: 50, y: 64, font: 6, text: 'maya' }),
				badge({ x: 50, y: 82 })
			];
		case 'split':
			return [
				rect({ x: 78, y: 50, w: 46, height: 130, radius: 0, opacity: 0.16 }),
				avatar({ x: 80, y: 50, size: 24 }),
				title({ x: 8, y: 40, anchor: 'left', align: 'left', font: 5.4, maxw: 56 }),
				subtitle({ x: 8, y: 58, anchor: 'left', align: 'left', font: 2.6, maxw: 56 })
			];
		case 'stacked':
			return [
				title({ x: 8, y: 34, anchor: 'left', align: 'left', font: 6.4, maxw: 84 }),
				subtitle({ x: 8, y: 55, anchor: 'left', align: 'left', font: 2.8, maxw: 78 }),
				badge({ x: 8, y: 74, anchor: 'left', text: 'New member' })
			];
		case 'centered':
		default:
			return [
				avatar({ x: 50, y: 31, size: 19 }),
				title({ x: 50, y: 62, font: 6.2 }),
				subtitle({ x: 50, y: 78 })
			];
	}
}
