// A chart or strip that reads the moment under the pointer, made for fingers too (bug 182). A sideways slide reads it
// and an upright one still scrolls the page (touch-action: pan-y), a long press after a touch opens no browser menu
// (around a status strip is a link, and Android offers "open in new tab, share" for it), and nothing gets selected.
import type { Action } from 'svelte/action';

export const scrub: Action<HTMLElement> = (node) => {
	let touch = false;
	const down = (e: PointerEvent) => (touch = e.pointerType !== 'mouse');
	const menu = (e: Event) => touch && e.preventDefault();
	node.style.touchAction = 'pan-y';
	node.style.userSelect = 'none';
	node.style.setProperty('-webkit-user-select', 'none');
	node.style.setProperty('-webkit-touch-callout', 'none');
	node.addEventListener('pointerdown', down, { capture: true });
	node.addEventListener('contextmenu', menu);
	return {
		destroy() {
			node.removeEventListener('pointerdown', down, { capture: true });
			node.removeEventListener('contextmenu', menu);
		}
	};
};

/** On touch the pointer "leaves" when the finger lifts: keep the reading until a tap elsewhere (the blur) clears it. */
export const mouseOnly = (f: () => void) => (e: PointerEvent) => {
	if (e.pointerType === 'mouse') f();
};
