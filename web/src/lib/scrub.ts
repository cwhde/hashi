// A chart or strip that reads the moment under the pointer, made for fingers too (bug 182). A sideways slide reads it
// and an upright one still scrolls the page (touch-action: pan-y), a long press after a touch opens no browser menu
// (around a status strip is a link, and Android offers "open in new tab, share" for it), and nothing gets selected.
//
// On touch a reading stays after the finger lifts, so there is one reading on the page at a time: touching another
// chart, or anything else, clears it (the parameter clears this element's reading), and so does a scroll that began on
// the chart. Waiting for a blur didn't work: a tap on a status strip never focuses it (the row's link is held back),
// so readings piled up on every strip touched.
import type { Action } from 'svelte/action';

let current: { node: HTMLElement; clear: () => void } | null = null;
let listening = false;

function elsewhere(e: PointerEvent) {
	if (current && !current.node.contains(e.target as Node)) {
		current.clear();
		current = null;
	}
}

export const scrub: Action<HTMLElement, () => void> = (node, clear) => {
	let touch = false;
	let off = clear;
	if (!listening) {
		document.addEventListener('pointerdown', elsewhere, { capture: true });
		listening = true;
	}
	const down = (e: PointerEvent) => {
		touch = e.pointerType !== 'mouse';
		current = { node, clear: () => off() };
	};
	const menu = (e: Event) => touch && e.preventDefault();
	// The browser took the finger for a scroll: that was no reading.
	const scrolled = () => {
		off();
		if (current?.node === node) current = null;
	};
	node.style.touchAction = 'pan-y';
	node.style.userSelect = 'none';
	node.style.setProperty('-webkit-user-select', 'none');
	node.style.setProperty('-webkit-touch-callout', 'none');
	node.addEventListener('pointerdown', down, { capture: true });
	node.addEventListener('contextmenu', menu);
	node.addEventListener('pointercancel', scrolled);
	return {
		update(next) {
			off = next;
		},
		destroy() {
			node.removeEventListener('pointerdown', down, { capture: true });
			node.removeEventListener('contextmenu', menu);
			node.removeEventListener('pointercancel', scrolled);
			if (current?.node === node) current = null;
		}
	};
};

/** On touch the pointer "leaves" when the finger lifts: keep the reading until the next touch elsewhere clears it. */
export const mouseOnly = (f: () => void) => (e: PointerEvent) => {
	if (e.pointerType === 'mouse') f();
};
