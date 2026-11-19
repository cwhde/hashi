// Finds words that render glued together (bug 148). Svelte 5 drops whitespace at the start and end of an element's or
// block's content, so `{label}<span>⏎ {method}</span>` renders "labelmethod". This flags a node whose content starts
// (or ends) with whitespace that will be dropped while the text next to it has none. Fix with an explicit {' '}.
import { globSync, readFileSync, writeFileSync } from 'node:fs';
import { parse } from 'svelte/compiler';

const BLOCKY = new Set([
	'div',
	'p',
	'ul',
	'ol',
	'li',
	'section',
	'header',
	'footer',
	'main',
	'nav',
	'aside',
	'article',
	'h1',
	'h2',
	'h3',
	'h4',
	'h5',
	'h6',
	'table',
	'thead',
	'tbody',
	'tr',
	'td',
	'th',
	'dl',
	'dt',
	'dd',
	'form',
	'fieldset',
	'pre',
	'svg',
	'br',
	'hr',
	'img',
	'input',
	'select',
	'textarea',
	'option'
]);
const fix = process.argv.includes('--fix');
const args = process.argv.slice(2).filter((a) => a !== '--fix');
const files = args.length ? args : globSync('src/**/*.svelte');

const inner = (n) => {
	if (n.type === 'IfBlock') return n.consequent?.nodes ?? [];
	if (n.type === 'EachBlock' || n.type === 'KeyBlock') return n.body?.nodes ?? [];
	if (n.type === 'RegularElement' || n.type === 'Component')
		return BLOCKY.has(n.name) ? null : n.fragment.nodes;
	return null;
};
// Content that starts with whitespace Svelte drops: a leading text node with something after it, or a first child
// that itself starts that way.
const startsWs = (nodes) => {
	const f = nodes?.[0];
	if (!f) return false;
	if (f.type === 'Text') return /^\s/.test(f.data) && (nodes.length > 1 || /\S/.test(f.data));
	return startsWs(inner(f));
};
const endsWs = (nodes) => {
	const l = nodes?.at(-1);
	if (!l) return false;
	if (l.type === 'Text') return /\s$/.test(l.data) && (nodes.length > 1 || /\S/.test(l.data));
	return endsWs(inner(l));
};
const space = (n) =>
	n.type === 'ExpressionTag' && n.expression.type === 'Literal' && /^\s+$/.test(n.expression.value);
const textEnd = (n) =>
	(n.type === 'ExpressionTag' && !space(n)) ||
	(n.type === 'Text' ? /\S$/.test(n.data) : inner(n) !== null);
const textStart = (n) =>
	(n.type === 'ExpressionTag' && !space(n)) ||
	(n.type === 'Text' ? /^\S/.test(n.data) : inner(n) !== null);

let found = 0;
for (const f of files) {
	const src = readFileSync(f, 'utf8');
	const line = (pos) => src.slice(0, pos).split('\n').length;
	const inserts = []; // offsets where {' '} goes: the start or end of the content that loses its space
	const walk = (nodes) => {
		if (!nodes) return;
		for (let i = 0; i < nodes.length; i++) {
			const n = nodes[i];
			const prev = nodes[i - 1],
				next = nodes[i + 1];
			if (prev && textEnd(prev) && startsWs(inner(n))) {
				console.log(
					`${f}:${line(n.start)}: the space at the start of this ${n.name ?? n.type} is dropped`
				);
				inserts.push(inner(n)[0].start);
				found++;
			}
			if (next && textStart(next) && endsWs(inner(n))) {
				console.log(
					`${f}:${line(n.end)}: the space at the end of this ${n.name ?? n.type} is dropped`
				);
				inserts.push(inner(n).at(-1).end);
				found++;
			}
			for (const k of [
				'fragment',
				'consequent',
				'alternate',
				'body',
				'fallback',
				'pending',
				'then',
				'catch'
			])
				if (n[k]?.nodes) walk(n[k].nodes);
		}
	};
	walk(parse(src, { modern: true }).fragment.nodes);
	if (fix && inserts.length) {
		let out = src;
		for (const at of [...new Set(inserts)].sort((a, b) => b - a))
			out = out.slice(0, at) + "{' '}" + out.slice(at);
		writeFileSync(f, out);
	}
}
if (found && !fix) {
	console.log(`${found} places where words would render glued together. Add an explicit {' '}.`);
	process.exit(1);
}
