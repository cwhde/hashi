import { api } from '$lib/api/client';

// Built-in themes (10-ui.md §2). The palette lives in routes/layout.css; this module only switches it.
export type ThemeId = 'graphite' | 'linen' | 'verdant' | 'violet';

// The saved theme is the database's; the browser's copy only avoids a flash of the wrong theme before first paint.
// Swatches are rendered by setting data-theme on a small element, so no colour is duplicated here.
export const themes: { id: ThemeId; name: string; mode: 'dark' | 'light' }[] = [
	{ id: 'graphite', name: 'Graphite', mode: 'dark' },
	{ id: 'linen', name: 'Warm Linen', mode: 'light' },
	{ id: 'verdant', name: 'Verdant', mode: 'dark' },
	{ id: 'violet', name: 'Violet Night', mode: 'dark' }
];

const KEY = 'hashi.theme';

function initial(): ThemeId {
	if (typeof document === 'undefined') return 'graphite';
	const t = document.documentElement.dataset.theme as ThemeId | undefined;
	return themes.some((x) => x.id === t) ? (t as ThemeId) : 'graphite';
}

class ThemeState {
	current = $state<ThemeId>(initial());

	set(id: ThemeId) {
		const t = themes.find((x) => x.id === id);
		if (!t) return;
		this.current = id;
		document.documentElement.dataset.theme = id;
		document.documentElement.dataset.mode = t.mode;
		try {
			localStorage.setItem(KEY, id);
		} catch {
			// private mode or blocked storage: the theme still applies for this page view
		}
	}

	/** Applies a theme and saves it, so other browsers, the public pages and alert mails follow. */
	async save(id: ThemeId) {
		this.set(id);
		const r = await api.GET('/api/settings/appearance');
		if (r.data) await api.PUT('/api/settings/appearance', { body: { ...r.data, theme: id } });
	}

	/** Takes the saved theme, for a browser that last saw another one. */
	async sync() {
		const r = await api.GET('/api/settings/appearance');
		if (r.data && r.data.theme !== this.current) this.set(r.data.theme as ThemeId);
	}
}

export const theme = new ThemeState();
