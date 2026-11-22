// Mock-only live sorting for the layout editor (07 §8.1), iOS home screen / Heimdall style: the dragged item lifts and
// follows the pointer, the others slide out of its way while it moves, and it settles into its place on release.
// Pointer events, so mouse, pen and touch behave the same; on touch only the grip starts a drag, so the list still scrolls.
// The ADR decides whether the real build keeps this approach or takes a library.
import { flip } from 'svelte/animate';
import type { Action } from 'svelte/action';

export type Kind = 'item' | 'group';
export type Hit = { id?: string; group?: string; after: boolean };
export interface DragHost {
	start(kind: Kind, id: string): void;
	over(kind: Kind, id: string, hit: Hit): void;
	end(): void;
}
type Params = {
	host: DragHost;
	kind: Kind;
	id: string;
	/** Only a pointer down inside this selector starts a drag (always for groups; on touch for items). */
	handle?: string;
	handleOnly?: boolean;
};

/** Slides the others out of the way; the dragged one follows the pointer instead. */
export function slide(node: Element, rects: { from: DOMRect; to: DOMRect }) {
	if ((node as HTMLElement).dataset.dragging) return { duration: 0 };
	return flip(node, rects, { duration: 180 });
}

export const sortable: Action<HTMLElement, Params> = (node, initial) => {
	let p = initial;
	let start: { x: number; y: number; pid: number } | null = null;
	let active = false;
	let off = { x: 0, y: 0 };
	let last = { x: 0, y: 0 };

	// Where the node sits without the drag's transform, so it can follow the pointer as the list reorders under it.
	function base() {
		const r = node.getBoundingClientRect();
		const m = new DOMMatrixReadOnly(getComputedStyle(node).transform);
		return { left: r.left - m.e, top: r.top - m.f };
	}
	/** An element's place in the layout, without any transform (a slide in progress, or the drag itself). */
	function slot(el: HTMLElement) {
		const r = el.getBoundingClientRect();
		const m = new DOMMatrixReadOnly(getComputedStyle(el).transform);
		return new DOMRect(r.left - m.e, r.top - m.f, r.width, r.height);
	}
	function place() {
		const b = base();
		node.style.transform = `translate(${last.x - off.x - b.left}px, ${last.y - off.y - b.top}px) scale(1.03)`;
	}

	// A grip drags at once. Elsewhere, a mouse drags after 5 px; a finger has to hold still for a moment first (as on
	// a phone's home screen), so a swipe still scrolls the page. Controls inside an item keep their own clicks, except a
	// button marked `data-sort-body` that is the whole item (a card in the preview).
	let hold: ReturnType<typeof setTimeout> | undefined;
	const block = (ev: Event) => active && ev.preventDefault();

	function down(e: PointerEvent) {
		if (e.button !== 0) return;
		const t = e.target as Element;
		if (t.closest('[data-sort-id]') !== node) return; // a nested sortable started it
		const inHandle = !!p.handle && !!t.closest(p.handle);
		if (p.handleOnly && !inHandle) return;
		const control = t.closest('button, input, a[href], [role="checkbox"]');
		if (!inHandle && control && !control.hasAttribute('data-sort-body')) return;
		start = { x: e.clientX, y: e.clientY, pid: e.pointerId };
		last = { x: e.clientX, y: e.clientY };
		if (e.pointerType !== 'mouse' && !inHandle) hold = setTimeout(() => start && begin(), 350);
		window.addEventListener('pointermove', move, { passive: false });
		window.addEventListener('pointerup', up);
		window.addEventListener('pointercancel', up);
		window.addEventListener('touchmove', block, { passive: false });
		window.addEventListener('contextmenu', block);
	}

	function begin() {
		if (!start || active) return;
		active = true;
		const b = base();
		off = { x: start.x - b.left, y: start.y - b.top };
		node.dataset.dragging = '1';
		node.style.zIndex = '30';
		node.style.position = 'relative';
		node.style.pointerEvents = 'none';
		node.style.boxShadow = '0 12px 28px -8px rgb(0 0 0 / 0.45)';
		document.body.style.userSelect = 'none';
		navigator.vibrate?.(8);
		p.host.start(p.kind, p.id);
		place();
	}

	function move(e: PointerEvent) {
		if (!start || e.pointerId !== start.pid) return;
		last = { x: e.clientX, y: e.clientY };
		if (!active) {
			const far = Math.hypot(last.x - start.x, last.y - start.y);
			if (hold) {
				if (far > 8) up(); // moved before the hold: a scroll, not a drag
				return;
			}
			if (far < 5) return;
			begin();
		}
		e.preventDefault();
		place();
		hit();
		requestAnimationFrame(place);
	}

	// Hit testing uses each item's slot (where it is going), not where it is drawn: while the others slide, the drawn
	// place lags behind, and testing it made the dragged item swap and swap back under a still pointer.
	function hit() {
		const inside = (r: DOMRect) =>
			last.x >= r.left && last.x < r.right && last.y >= r.top && last.y < r.bottom;
		const item = [
			...document.querySelectorAll<HTMLElement>(`[data-sort-kind="${p.kind}"][data-sort-id]`)
		].find((el) => el !== node && el.offsetParent !== null && inside(slot(el)));
		if (item) {
			const r = slot(item);
			const after =
				item.dataset.sortAxis === 'x'
					? last.x > r.left + r.width / 2
					: last.y > r.top + r.height / 2;
			p.host.over(p.kind, p.id, { id: item.dataset.sortId, group: item.dataset.sortGroup, after });
			return;
		}
		// Over a group's empty space: join its end, but only past its last item (not in the gap the dragged one left).
		const els = document.elementsFromPoint(last.x, last.y) as HTMLElement[];
		const zone = els.find((el) => el.dataset.sortZone === p.kind);
		if (!zone) return;
		const items = [...zone.querySelectorAll<HTMLElement>(`[data-sort-kind="${p.kind}"]`)].filter(
			(el) => el !== node && el.closest('[data-sort-zone]') === zone
		);
		const tail = items.at(-1);
		const end = tail && slot(tail);
		if (!end || last.y > end.bottom || (last.y > end.top && last.x > end.right))
			p.host.over(p.kind, p.id, { group: zone.dataset.sortGroup, after: true });
	}

	function release() {
		clearTimeout(hold);
		hold = undefined;
		start = null;
		window.removeEventListener('pointermove', move);
		window.removeEventListener('pointerup', up);
		window.removeEventListener('pointercancel', up);
		window.removeEventListener('touchmove', block);
		window.removeEventListener('contextmenu', block);
	}

	function up() {
		release();
		if (!active) return;
		active = false;
		// Settle into the slot, then hand back to the layout.
		node.style.transition = 'transform 160ms ease-out, box-shadow 160ms';
		node.style.transform = '';
		node.style.boxShadow = '';
		document.body.style.userSelect = '';
		setTimeout(() => {
			node.style.transition = '';
			node.style.zIndex = '';
			node.style.position = '';
			node.style.pointerEvents = '';
			delete node.dataset.dragging;
		}, 170);
		// The release after a drag is not a click.
		const eat = (ev: Event) => ev.stopPropagation();
		window.addEventListener('click', eat, { capture: true, once: true });
		setTimeout(() => window.removeEventListener('click', eat, { capture: true }), 60);
		p.host.end();
	}

	node.addEventListener('pointerdown', down);
	return {
		update(next) {
			p = next;
		},
		destroy() {
			node.removeEventListener('pointerdown', down);
			release();
		}
	};
};
