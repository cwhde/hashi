// Where sign-in sends the admin back to (bug 97). Both redirects to the sign-in page build it here, so the open tab
// and any other query survive; the sign-in page reads it back through safeNext.

type Here = { pathname: string; search: string };

/** The sign-in address that returns to here; ended says the session ran out rather than never existed. */
export function signInHref(here: Here, ended = false): string {
	const next = encodeURIComponent(here.pathname + here.search);
	return `/login?${ended ? 'ended=1&' : ''}next=${next}`;
}

/** A path inside this app, or the default page: never another origin. Browsers read `/\` like `//`. */
export function safeNext(raw: string | null, fallback = '/overview'): string {
	const n = raw ?? '';
	return n.startsWith('/') && !n.startsWith('//') && !n.startsWith('/\\') ? n : fallback;
}
