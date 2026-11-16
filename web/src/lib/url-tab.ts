// A tabbed page keeps its open tab in the address (?tab=…), so a reload, back or a shared link opens the same tab
// (bug 41). The first tab is the default and leaves no parameter; other query parameters stay as they are.

/** What these helpers read from an address; SvelteKit's page.url is a read-only URL. */
type Address = { readonly href: string; readonly searchParams: Pick<URLSearchParams, 'get'> };

/** The tab the address asks for, or the first one when it names none or an unknown one. */
export function tabFrom(url: Address, tabs: readonly string[]): string {
	const t = url.searchParams.get('tab');
	return t && tabs.includes(t) ? t : tabs[0]!;
}

/** The address with tab t in it: path, other parameters and hash unchanged. */
export function hrefWithTab(url: Address, tabs: readonly string[], t: string): string {
	const u = new URL(url.href);
	if (t === tabs[0]) u.searchParams.delete('tab');
	else u.searchParams.set('tab', t);
	return u.pathname + u.search + u.hash;
}
