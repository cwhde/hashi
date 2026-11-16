import { beforeNavigate } from '$app/navigation';

/** Remembers a form's saved state; `dirty` says whether what is on screen differs from it (bug 40). */
export class Draft {
	#saved = $state<string | null>(null);
	#read: () => unknown;

	constructor(read: () => unknown) {
		this.#read = read;
	}

	/** Call after loading or saving: what is on screen now counts as saved. */
	mark() {
		this.#saved = JSON.stringify(this.#read());
	}

	get dirty() {
		return this.#saved !== null && JSON.stringify(this.#read()) !== this.#saved;
	}
}

/**
 * Asks before leaving the page while something is unsaved: a link, back, or closing the tab (the browser then asks in
 * its own words). Switching a tab or a Settings category stays on the page and keeps the draft, so it doesn't ask.
 * Call it while the component starts.
 */
export function guardUnsaved(dirty: () => boolean) {
	beforeNavigate((nav) => {
		if (!dirty()) return;
		if (nav.to && nav.from && nav.to.url.pathname === nav.from.url.pathname) return;
		if (nav.type === 'leave') {
			nav.cancel();
			return;
		}
		if (!confirm('This page has changes that are not saved. Leave without saving them?'))
			nav.cancel();
	});
}
