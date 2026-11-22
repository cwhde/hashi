// Mock of the badge renderer (07 §7.3): the exact SVG the status listener will send, so the service page can show it
// as an <img>, the way a README does. A badge is a file shown on other sites (light and dark), so its colours are fixed
// in the image, not Hashi's theme tokens; the real renderer (Go) uses the same numbers. Text widths come from a width
// table for 11 px Helvetica/Arial, so no font is loaded and the server needs no font files.

export type Tone = 'up' | 'degraded' | 'down' | 'unknown';

const tones: Record<Tone, string> = {
	up: '#1a7f37', // white text 5.1:1
	degraded: '#9a6700', // 5.0:1
	down: '#cf222e', // 5.4:1
	unknown: '#6e7781' // 4.6:1
};
const labelFill = '#3d444d';

// Advance widths at 11 px, from Arial's metrics; anything missing counts as a lowercase letter.
const narrow = 'iljI.,:;|!\'"';
const thin = 'frt ()[]-/';
const wide = 'mwMW%@';
function width(s: string, bold = false) {
	let w = 0;
	for (const c of s) {
		if (narrow.includes(c)) w += 2.6;
		else if (thin.includes(c)) w += 3.5;
		else if (wide.includes(c)) w += 9.2;
		else if (c >= '0' && c <= '9') w += 6.1;
		else if (c >= 'A' && c <= 'Z') w += 7.2;
		else w += 5.9;
	}
	return Math.ceil(w * (bold ? 1.07 : 1));
}
const esc = (s: string) =>
	s.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]!);

/** One badge: a dark label, the value on its state colour, and for `status` a dot before the value. */
export function badgeSvg(label: string, value: string, tone: Tone, dot = false) {
	const pad = 7;
	const lw = width(label) + pad * 2;
	const vText = width(value, true);
	const vw = vText + pad * 2 + (dot ? 10 : 0);
	const w = lw + vw;
	const vx = lw + pad + (dot ? 10 : 0);
	const text = (x: number, s: string, weight: number) =>
		`<text x="${x}" y="15" fill="#010101" fill-opacity=".3" font-weight="${weight}">${esc(s)}</text>` +
		`<text x="${x}" y="14" font-weight="${weight}">${esc(s)}</text>`;
	return (
		`<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="20" role="img" aria-label="${esc(label)}: ${esc(value)}">` +
		`<title>${esc(label)}: ${esc(value)}</title>` +
		`<linearGradient id="s" x2="0" y2="100%"><stop offset="0" stop-color="#fff" stop-opacity=".1"/>` +
		`<stop offset="1" stop-opacity=".1"/></linearGradient>` +
		`<clipPath id="r"><rect width="${w}" height="20" rx="4" fill="#fff"/></clipPath>` +
		`<g clip-path="url(#r)"><rect width="${lw}" height="20" fill="${labelFill}"/>` +
		`<rect x="${lw}" width="${vw}" height="20" fill="${tones[tone]}"/>` +
		`<rect width="${w}" height="20" fill="url(#s)"/>` +
		`<rect x=".5" y=".5" width="${w - 1}" height="19" rx="3.5" fill="none" stroke="#000" stroke-opacity=".15"/></g>` +
		(dot ? `<circle cx="${lw + pad + 3}" cy="10" r="3" fill="#fff" fill-opacity=".95"/>` : '') +
		`<g fill="#fff" font-family="Helvetica,Arial,sans-serif" font-size="11" text-rendering="geometricPrecision">` +
		text(pad, label, 500) +
		text(vx, value, 700) +
		`</g></svg>`
	);
}

export const badgeUri = (svg: string) =>
	`data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`;
