import { beforeNavigate, goto } from '$app/navigation';

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

/** The leave question the shell shows in the app's own dialog (bug 108); `to` is where the owner wanted to go. */
class LeaveQuestion {
	to = $state<URL | null>(null);
	#pass = false;

	ask(to: URL) {
		this.to = to;
	}
	/** Leave without saving: go where the owner wanted, without asking again. */
	async leave() {
		const to = this.to;
		this.to = null;
		if (!to) return;
		this.#pass = true;
		try {
			await goto(to.pathname + to.search + to.hash);
		} finally {
			this.#pass = false;
		}
	}
	stay() {
		this.to = null;
	}
	get passing() {
		return this.#pass;
	}
}
export const leaveQuestion = new LeaveQuestion();

/**
 * Asks before leaving the page while something is unsaved: a link or back asks in the app's own dialog; closing the tab
 * makes the browser ask in its own words (a page can't show its own there). Switching a tab or a Settings category stays
 * on the page and keeps the draft, so it doesn't ask. Call it while the component starts.
 */
export function guardUnsaved(dirty: () => boolean) {
	beforeNavigate((nav) => {
		if (!dirty() || leaveQuestion.passing) return;
		if (nav.to && nav.from && nav.to.url.pathname === nav.from.url.pathname) return;
		nav.cancel();
		if (nav.type !== 'leave' && nav.to) leaveQuestion.ask(nav.to.url);
	});
}
