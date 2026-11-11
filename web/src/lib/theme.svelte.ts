import { api, type Schemas } from '$lib/api/client';
import { contrast, luminance } from '$lib/design/contrast';

// Built-in themes (10-ui.md §2). Their palette lives in routes/layout.css; this module switches it. A custom theme sets
// the palette roles (--h-*) inline from the admin's six colours; layout.css derives every semantic token from them.
export type ThemeId = 'graphite' | 'linen' | 'verdant' | 'violet' | 'custom';
export type CustomTheme = Schemas['CustomTheme'];
type Appearance = Pick<
	Schemas['AppearanceSettings'],
	'theme' | 'custom_theme' | 'reduce_transparency'
>;

// The saved theme is the database's; the browser's copy only avoids a flash of the wrong theme before first paint.
// Swatches are rendered by setting data-theme on a small element, so no colour is duplicated here.
export const themes: { id: Exclude<ThemeId, 'custom'>; name: string; mode: 'dark' | 'light' }[] = [
	{ id: 'graphite', name: 'Graphite', mode: 'dark' },
	{ id: 'linen', name: 'Warm Linen', mode: 'light' },
	{ id: 'verdant', name: 'Verdant', mode: 'dark' },
	{ id: 'violet', name: 'Violet Night', mode: 'dark' }
];

/** Text and muted text need this contrast on the surface, as the server checks (settings.MinTextContrast). */
export const minContrast = 4.5;

const KEY = 'hashi.theme';
const VARS = 'hashi.theme.vars'; // the custom theme's inline variables, for app.html's first paint
const SOLID = 'hashi.solid';

const radius = { sharp: '0.25rem', default: '0.75rem', round: '1.25rem' } as const;

const mix = (a: string, b: string, pct: number) => `color-mix(in oklab, ${a} ${100 - pct}%, ${b})`;

/** Is a custom theme light (its canvas bright)? Light themes get the darker status text shades. */
export const isLight = (c: CustomTheme) => luminance(c.canvas) > 0.4;

/** Black or white, whichever reads better on the accent (the server's AccentText). */
export const accentText = (accent: string) =>
	contrast('#000000', accent) >= contrast('#ffffff', accent) ? '#000000' : '#ffffff';

/** The palette roles and tokens a custom theme sets; the rest of layout.css derives from them. */
export function customVars(c: CustomTheme): Record<string, string> {
	const vars: Record<string, string> = {
		'--h-canvas': c.canvas,
		'--h-shell': mix(c.canvas, c.surface, 50),
		'--h-surface': c.surface,
		'--h-surface-2': mix(c.surface, c.text, 5),
		'--h-elevated': mix(c.surface, c.text, 9),
		'--h-border': c.border,
		'--h-text': c.text,
		'--h-muted': c.muted,
		'--h-accent': c.accent,
		'--h-accent-text': accentText(c.accent),
		'--h-accent-hover': mix(c.accent, c.text, 15),
		'--h-focus': mix(c.accent, c.text, 35),
		'--radius': radius[c.roundness]
	};
	(c.charts ?? []).forEach((col, i) => (vars[`--chart-${i + 1}`] = col));
	return vars;
}

/** What a theme check says about a draft: the problems that keep it from being saved. */
export function themeProblems(c: CustomTheme): string[] {
	const out: string[] = [];
	for (const [k, v] of Object.entries(c)) {
		if (k !== 'charts' && k !== 'roundness' && !/^#[0-9a-f]{6}$/i.test(v as string))
			out.push(`${k} must be a colour like #1a2b3c`);
	}
	if (out.length) return out;
	const pairs: [string, string, string][] = [
		['Text on the surface', c.text, c.surface],
		['Muted text on the surface', c.muted, c.surface],
		['Text on the canvas', c.text, c.canvas]
	];
	for (const [what, fg, bg] of pairs) {
		const r = contrast(fg, bg);
		if (r < minContrast)
			out.push(`${what} is too faint (${r.toFixed(1)}:1, needs ${minContrast}:1)`);
	}
	return out;
}

function store(k: string, v: string | null) {
	try {
		if (v === null) localStorage.removeItem(k);
		else localStorage.setItem(k, v);
	} catch {
		// private mode or blocked storage: the theme still applies for this page view
	}
}

function initial(): ThemeId {
	if (typeof document === 'undefined') return 'graphite';
	const t = document.documentElement.dataset.theme as ThemeId | undefined;
	return t === 'custom' || themes.some((x) => x.id === t) ? (t as ThemeId) : 'graphite';
}

class ThemeState {
	current = $state<ThemeId>(initial());
	custom = $state<CustomTheme | null>(null);
	solid = $state(false);

	/** Applies a theme in this browser. 'custom' needs the custom palette (apply() brings it). */
	set(id: ThemeId) {
		const root = document.documentElement;
		for (const k of Object.keys(customVars(fallbackCustom))) root.style.removeProperty(k);
		for (let i = 1; i <= 5; i++) root.style.removeProperty(`--chart-${i}`);
		if (id === 'custom' && this.custom) {
			const vars = customVars(this.custom);
			for (const [k, v] of Object.entries(vars)) root.style.setProperty(k, v);
			root.dataset.theme = 'custom';
			root.dataset.mode = isLight(this.custom) ? 'light' : 'dark';
			root.style.colorScheme = root.dataset.mode;
			this.current = 'custom';
			store(KEY, 'custom');
			store(VARS, JSON.stringify({ vars, mode: root.dataset.mode }));
			return;
		}
		// An unknown id, or custom without a palette, falls back to Graphite (10-ui §2).
		const t = themes.find((x) => x.id === id) ?? themes[0]!;
		root.style.removeProperty('color-scheme');
		this.current = t.id;
		root.dataset.theme = t.id;
		root.dataset.mode = t.mode;
		store(KEY, t.id);
		store(VARS, null);
	}

	/** Solid backgrounds instead of translucent overlays (Settings → Appearance → Comfort). */
	setSolid(on: boolean) {
		this.solid = on;
		if (on) document.documentElement.dataset.solid = '';
		else delete document.documentElement.dataset.solid;
		store(SOLID, on ? '1' : null);
	}

	/** Applies the appearance as the server has it (pages, the public pages, after a save). */
	apply(a: Appearance) {
		this.custom = a.custom_theme ?? null;
		this.set(a.theme as ThemeId);
		if (a.reduce_transparency !== undefined) this.setSolid(a.reduce_transparency);
	}

	/** Applies a theme and saves it, so other browsers, the public pages and alert mails follow. */
	async save(id: ThemeId) {
		this.set(id);
		const r = await api.GET('/api/settings/appearance');
		if (r.data) await api.PUT('/api/settings/appearance', { body: { ...r.data, theme: id } });
	}

	/** Takes the saved appearance, for a browser that last saw another one. */
	async sync() {
		const r = await api.GET('/api/settings/appearance');
		if (r.data) this.apply(r.data);
	}
}

const fallbackCustom: CustomTheme = {
	canvas: '#000000',
	surface: '#000000',
	text: '#ffffff',
	muted: '#ffffff',
	border: '#000000',
	accent: '#000000',
	roundness: 'default'
};

export const theme = new ThemeState();
