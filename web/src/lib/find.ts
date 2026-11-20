// A search result opens the exact place (bug 4): `?find=<key>` makes the page scroll to that row or setting and mark it
// for a moment. A row says what finds it with `data-find`; a setting is found by its label's text.

const mark = ['ring-2', 'ring-ring', 'ring-offset-2', 'ring-offset-background', 'rounded-md'];

function target(key: string): HTMLElement | null {
	const row = document.querySelector<HTMLElement>(`[data-find="${CSS.escape(key)}"]`);
	if (row) return row;
	const want = key.trim().toLowerCase();
	for (const el of document.querySelectorAll<HTMLElement>(
		'main label, main dt, main h2, main h3, main span, main p'
	)) {
		if (el.closest('[hidden], [aria-hidden="true"]')) continue;
		if (el.textContent?.trim().toLowerCase() === want) return el;
	}
	return null;
}

/**
 * Waits up to `waitMs` for the element `key` names (pages load their rows after mounting), then scrolls it into view,
 * marks it and moves focus to its control. Returns whether it was found.
 */
export function reveal(key: string, waitMs = 5000): Promise<boolean> {
	const until = performance.now() + waitMs;
	return new Promise((done) => {
		const look = () => {
			const el = target(key);
			if (!el) {
				if (performance.now() < until) requestAnimationFrame(look);
				else done(false);
				return;
			}
			const reduce = matchMedia('(prefers-reduced-motion: reduce)').matches;
			el.scrollIntoView({ block: 'center', behavior: reduce ? 'auto' : 'smooth' });
			const control =
				el instanceof HTMLLabelElement && el.control
					? el.control
					: el.matches('a, button, input, select, textarea, [tabindex]')
						? el
						: null;
			control?.focus({ preventScroll: true });
			el.classList.add(...mark);
			setTimeout(() => el.classList.remove(...mark), 2500);
			done(true);
		};
		look();
	});
}

/** The page's `?find=` key, if any. */
export const findKey = (url: { readonly searchParams: Pick<URLSearchParams, 'get'> }) =>
	url.searchParams.get('find');
