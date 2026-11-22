// Live sorting for the Public pages editor (07 §8.1, ADR 0004), as on a phone's home screen: the dragged item lifts and
// follows the pointer, the others slide out of its way while it moves, and it settles into its place on release.
// Pointer events, so mouse, pen and touch behave the same. The order lives in the host's Svelte state; this action only
// reports where the pointer is (`over`), and keyed `{#each}` blocks with `animate:slide` draw the result.
//
// One drag at a time lives here, not in the element: an item that moves into another group is drawn by that group's
// `{#each}`, so Svelte replaces its element mid-drag. The new element (same scope, kind and id) takes the drag over;
// without that the old element's cleanup dropped the drag and the release never reached the host.
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
	/** Which view the element belongs to ("outline", "preview"): a drag only moves to an element of its own view. */
	scope?: string;
	/** Only a pointer down inside this selector starts a drag (always for groups; on touch for items). */
	handle?: string;
	handleOnly?: boolean;
};

/** Slides the others out of the way; the dragged one follows the pointer instead. */
export function slide(node: Element, rects: { from: DOMRect; to: DOMRect }) {
	if ((node as HTMLElement).dataset.dragging) return { duration: 0 };
	return flip(node, rects, { duration: 180 });
}

/** The element's place in the layout, without any transform (a slide in progress, or the drag itself). */
function slot(el: HTMLElement) {
	const r = el.getBoundingClientRect();
	const m = new DOMMatrixReadOnly(getComputedStyle(el).transform);
	return new DOMRect(r.left - m.e, r.top - m.f, r.width, r.height);
}

const LIFT = '0 12px 28px -8px rgb(0 0 0 / 0.45)';

class Drag {
	node: HTMLElement | null;
	active = false;
	off = { x: 0, y: 0 };
	last: { x: number; y: number };
	hold: ReturnType<typeof setTimeout> | undefined;

	constructor(
		node: HTMLElement,
		public p: Params,
		public start: { x: number; y: number; pid: number },
		touchHold: boolean
	) {
		this.node = node;
		this.last = { x: start.x, y: start.y };
		window.addEventListener('pointermove', this.move, { passive: false });
		window.addEventListener('pointerup', this.up);
		window.addEventListener('pointercancel', this.up);
		window.addEventListener('contextmenu', this.block);
		if (touchHold) this.hold = setTimeout(() => this.begin(), 350);
	}

	matches(p: Params) {
		return p.kind === this.p.kind && p.id === this.p.id && (p.scope ?? '') === (this.p.scope ?? '');
	}

	private lift(n: HTMLElement) {
		n.dataset.dragging = '1';
		n.style.zIndex = '30';
		n.style.position = 'relative';
		n.style.pointerEvents = 'none';
		n.style.boxShadow = LIFT;
	}
	private place() {
		const n = this.node;
		if (!n) return;
		const b = slot(n);
		n.style.transform = `translate(${this.last.x - this.off.x - b.left}px, ${this.last.y - this.off.y - b.top}px) scale(1.03)`;
	}

	begin() {
		if (this.active || !this.node) return;
		this.active = true;
		this.hold = undefined;
		const b = slot(this.node);
		this.off = { x: this.start.x - b.left, y: this.start.y - b.top };
		this.lift(this.node);
		document.body.style.userSelect = 'none';
		navigator.vibrate?.(8);
		this.p.host.start(this.p.kind, this.p.id);
		this.place();
	}

	/** A new element for the dragged item (it moved into another group's list) takes over. */
	adopt(n: HTMLElement) {
		this.node = n;
		if (!this.active) return;
		this.lift(n);
		this.place();
	}
	/** The element went away; the drag goes on until the release, and a new element may take it over. */
	lose(n: HTMLElement) {
		if (this.node === n) this.node = null;
		if (!this.active) this.stop();
	}

	block = (ev: Event) => {
		if (this.active) ev.preventDefault();
	};

	move = (e: PointerEvent) => {
		if (e.pointerId !== this.start.pid) return;
		this.last = { x: e.clientX, y: e.clientY };
		if (!this.active) {
			const far = Math.hypot(this.last.x - this.start.x, this.last.y - this.start.y);
			if (this.hold) {
				if (far > 8) this.stop(); // moved before the hold: a scroll, not a drag
				return;
			}
			if (far < 5) return;
			this.begin();
		}
		e.preventDefault();
		this.place();
		this.hit();
		requestAnimationFrame(() => this.place());
	};

	// Hit testing uses each item's slot (where it is going), not where it is drawn: while the others slide, the drawn
	// place lags behind, and testing it made the dragged item swap and swap back under a still pointer.
	private hit() {
		const { kind, id, host } = this.p;
		const at = this.last;
		const inside = (r: DOMRect) =>
			at.x >= r.left && at.x < r.right && at.y >= r.top && at.y < r.bottom;
		const item = [
			...document.querySelectorAll<HTMLElement>(`[data-sort-kind="${kind}"][data-sort-id]`)
		].find((el) => el !== this.node && el.offsetParent !== null && inside(slot(el)));
		if (item) {
			const r = slot(item);
			const after =
				item.dataset.sortAxis === 'x' ? at.x > r.left + r.width / 2 : at.y > r.top + r.height / 2;
			host.over(kind, id, { id: item.dataset.sortId, group: item.dataset.sortGroup, after });
			return;
		}
		// Over a group's empty space: join its end, but only past its last item (not in the gap the dragged one left).
		const els = document.elementsFromPoint(at.x, at.y) as HTMLElement[];
		const zone = els.find((el) => el.dataset.sortZone === kind);
		if (!zone) return;
		const items = [...zone.querySelectorAll<HTMLElement>(`[data-sort-kind="${kind}"]`)].filter(
			(el) => el !== this.node && el.closest('[data-sort-zone]') === zone
		);
		const tail = items.at(-1);
		const end = tail && slot(tail);
		if (!end || at.y > end.bottom || (at.y > end.top && at.x > end.right))
			host.over(kind, id, { group: zone.dataset.sortGroup, after: true });
	}

	private stop() {
		clearTimeout(this.hold);
		this.hold = undefined;
		window.removeEventListener('pointermove', this.move);
		window.removeEventListener('pointerup', this.up);
		window.removeEventListener('pointercancel', this.up);
		window.removeEventListener('contextmenu', this.block);
		if (current === this) current = null;
	}

	up = () => {
		this.stop();
		if (!this.active) return;
		this.active = false;
		document.body.style.userSelect = '';
		// Settle into the slot, then hand back to the layout.
		const n = this.node;
		if (n) {
			n.style.transition = 'transform 160ms ease-out, box-shadow 160ms';
			n.style.transform = '';
			n.style.boxShadow = '';
			setTimeout(() => {
				n.style.transition = '';
				n.style.zIndex = '';
				n.style.position = '';
				n.style.pointerEvents = '';
				delete n.dataset.dragging;
			}, 170);
		}
		// The release after a drag is not a click.
		const eat = (ev: Event) => ev.stopPropagation();
		window.addEventListener('click', eat, { capture: true, once: true });
		setTimeout(() => window.removeEventListener('click', eat, { capture: true }), 60);
		this.p.host.end();
	};
}

let current: Drag | null = null;

export const sortable: Action<HTMLElement, Params> = (node, initial) => {
	let p = initial;
	if (current?.matches(p)) current.adopt(node);

	// A grip drags at once. Elsewhere, a mouse drags after 5 px; a finger has to hold still for a moment first (as on
	// a phone's home screen), so a swipe still scrolls the page. Controls inside an item keep their own clicks, except a
	// button marked `data-sort-body` that is the whole item (a card in the preview).
	function down(e: PointerEvent) {
		if (e.button !== 0 || current) return;
		const t = e.target as Element;
		if (t.closest('[data-sort-id]') !== node) return; // a nested sortable started it
		const inHandle = !!p.handle && !!t.closest(p.handle);
		if (p.handleOnly && !inHandle) return;
		const control = t.closest('button, input, a[href], [role="checkbox"]');
		if (!inHandle && control && !control.hasAttribute('data-sort-body')) return;
		current = new Drag(
			node,
			p,
			{ x: e.clientX, y: e.clientY, pid: e.pointerId },
			e.pointerType !== 'mouse' && !inHandle
		);
	}

	// A finger's drag must not scroll the page. The browser decides whether a touch can be cancelled when the finger lands,
	// by whether a blocking touchmove listener is there already, so this one is on every item from the start (a listener
	// added on pointerdown came too late: the page scrolled and cancelled the drag). Touch events keep going to the element
	// the finger landed on even after Svelte replaced it, so the listener stays on the old element too.
	node.addEventListener(
		'touchmove',
		(ev) => current?.active && ev.cancelable && ev.preventDefault(),
		{
			passive: false
		}
	);
	node.addEventListener('pointerdown', down);
	return {
		update(next) {
			p = next;
			if (current?.node === node) current.p = next;
		},
		destroy() {
			node.removeEventListener('pointerdown', down);
			current?.lose(node);
		}
	};
};
