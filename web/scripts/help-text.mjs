// Counts the help sentences in the admin UI (10c §4, bug 93): page and panel descriptions and muted text. Empty-state
// lines ("No scripts yet.") and text inside a HelpTip popover are not help text on the page, so they don't count.
// Report only: `node scripts/help-text.mjs [--files]`. The 13.4 gate is below 40.
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';

const root = new URL('../src/', import.meta.url).pathname;
// The design mocks and the components only they use are not the app.
const skip = [
	'routes/design',
	'lib/components/ui',
	'lib/components/hashi/security/IpItemEditor.svelte',
	'lib/components/hashi/security/IpSetPicker.svelte'
];

function* svelteFiles(dir) {
	for (const name of readdirSync(dir)) {
		const p = join(dir, name);
		if (skip.some((s) => relative(root, p).startsWith(s))) continue;
		if (statSync(p).isDirectory()) yield* svelteFiles(p);
		else if (p.endsWith('.svelte')) yield p;
	}
}

// The text a reader sees: tags dropped, expressions shown as a placeholder word.
function visible(s) {
	// Expressions nest ({a ? `${b}` : c}): drop the innermost until none is left, then the tags.
	for (let prev = ''; prev !== s;) [prev, s] = [s, s.replace(/\{[^{}]*\}/g, 'x')];
	return s
		.replace(/<[^>]*>/g, ' ')
		.replace(/&[a-z]+;/g, ' ')
		.replace(/\s+/g, ' ')
		.trim();
}

// The inner text of each element whose class makes it muted, nested elements of the same tag included.
function mutedTexts(markup) {
	const out = [];
	const open = /<([a-zA-Z][\w.]*)\b[^>]*class=(?:"[^"]*|\{[^}]*)text-muted-foreground[^>]*>/g;
	let m;
	while ((m = open.exec(markup))) {
		const tag = m[1];
		let depth = 1;
		let i = open.lastIndex;
		const re = new RegExp(`<(/?)${tag.replace('.', '\\.')}\\b[^>]*?(/?)>`, 'g');
		re.lastIndex = i;
		let t;
		while (depth && (t = re.exec(markup))) {
			if (t[2]) continue;
			depth += t[1] ? -1 : 1;
			i = t.index;
		}
		out.push(visible(markup.slice(open.lastIndex, i)));
		open.lastIndex = i;
	}
	return out;
}

function texts(src) {
	const script = (src.match(/<script[^>]*>[\s\S]*?<\/script>/g) ?? []).join('\n');
	const markup = src
		.replace(/<script[^>]*>[\s\S]*?<\/script>/g, '')
		.replace(/<style[^>]*>[\s\S]*?<\/style>/g, '')
		.replace(/<HelpTip\b[\s\S]*?<\/HelpTip>/g, '');
	const out = [];
	// Descriptions and hints are help text whether or not they end in a full stop (bug 115: "They run in the order you
	// pick them" slipped through); muted text counts only as full sentences, since most of it is values and labels.
	for (const d of markup.matchAll(/\bdescription=(?:"([^"]*)"|\{['`]([^'`]*)['`]\})/g))
		out.push({ t: visible(d[1] ?? d[2]), explicit: true });
	for (const d of script.matchAll(/\b(?:description|hint|help):\s*['`"]([^'`"]{12,})['`"]/g))
		out.push({ t: visible(d[1]), explicit: true });
	out.push(...mutedTexts(markup).map((t) => ({ t, explicit: false })));
	return out;
}

const sentences = ({ t, explicit }) =>
	t
		.split(/(?<=[.!?])\s+/)
		.filter((s) => s.split(' ').length >= 3 && (explicit || /[.!?]$/.test(s)))
		.filter((s) => !/^(No|Nothing|None)\b/.test(s));

const counts = [];
for (const f of svelteFiles(root)) {
	const found = texts(readFileSync(f, 'utf8')).flatMap(sentences);
	if (found.length) counts.push({ file: relative(root, f), n: found.length, found });
}
counts.sort((a, b) => b.n - a.n);
const total = counts.reduce((s, c) => s + c.n, 0);
console.log(
	`help text: ${total} sentences in ${counts.length} files (10c §4; 13.4 gate: below 40)`
);
const all = process.argv.includes('--files');
for (const c of all ? counts : counts.slice(0, 10)) {
	console.log(`  ${String(c.n).padStart(3)}  ${c.file}`);
	if (process.argv.includes('--text')) for (const s of c.found) console.log(`         ${s}`);
}
