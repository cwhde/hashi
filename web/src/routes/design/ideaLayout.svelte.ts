// Mock state for the layout editor (07 §8.1): ONE order for both public pages. Every entry (service, DNS link, check) has a
// place in one group; each page shows the entries switched on for it and skips the rest, keeping the shared order.
import { entries, groupOrder, type Entry } from './ideas.ts';
import type { DragHost, Hit, Kind } from '#lib/sortable.ts';

export type Page = 'apps' | 'status';
export type Group = { name: string; ids: string[] };

const byName = (a: Entry, b: Entry) => a.name.localeCompare(b.name);

export class LayoutState implements DragHost {
	items = $state<Record<string, Entry>>(Object.fromEntries(entries.map((e) => [e.id, { ...e }])));
	groups = $state<Group[]>(
		[...groupOrder, 'Other'].map((g) => ({
			name: g,
			ids: entries
				.filter((e) => (g === 'Other' ? !e.group : e.group === g))
				.sort(byName)
				.map((e) => e.id)
		}))
	);
	selected = $state<string[]>([]);
	dragging = $state<{ kind: Kind; id: string } | null>(null);
	/** The last change, for the Undo toast: every change saves at once (07 §8.1). */
	last = $state<{ text: string; before: string } | null>(null);
	/** For screen readers: what the last move did. */
	said = $state('');

	private snap() {
		return JSON.stringify({ items: this.items, groups: this.groups });
	}
	/** Records a change before it happens; `text` is what the toast and screen readers say. */
	note(text: string) {
		this.last = { text, before: this.snap() };
		this.said = text;
	}
	undo() {
		if (!this.last) return;
		const v = JSON.parse(this.last.before) as { items: Record<string, Entry>; groups: Group[] };
		this.items = v.items;
		this.groups = v.groups;
		this.said = `Undone: ${this.last.text}`;
		this.last = null;
	}
	groupOf(id: string) {
		return this.groups.find((g) => g.ids.includes(id))!;
	}
	/** Keyboard move (Alt+Up/Down or the Move menu): one place up or down, across into the next group at its ends. */
	shift(id: string, by: -1 | 1) {
		const gi = this.groups.findIndex((g) => g.ids.includes(id));
		const g = this.groups[gi]!;
		const i = g.ids.indexOf(id);
		const name = this.items[id]!.name;
		if ((by < 0 && i > 0) || (by > 0 && i < g.ids.length - 1)) {
			this.note(`${name} moved ${by < 0 ? 'up' : 'down'} in ${g.name}`);
			const ids = [...g.ids];
			[ids[i], ids[i + by]] = [ids[i + by]!, ids[i]!];
			g.ids = ids;
			return;
		}
		const to = this.groups[gi + by];
		if (!to) return;
		this.note(`${name} moved to ${to.name}`);
		g.ids = g.ids.filter((x) => x !== id);
		if (by < 0) to.ids = [...to.ids, id];
		else to.ids = [id, ...to.ids];
		this.items[id]!.group = to.name === 'Other' ? '' : to.name;
	}
	/** The selection bar's switches: on or off for every selected entry where it is possible. */
	setSelected(p: Page, on: boolean) {
		this.note(`${this.selected.length} entries ${on ? 'shown on' : 'hidden from'} the ${p} page`);
		for (const id of this.selected) {
			const e = this.items[id]!;
			if (this.possible(e, p)) e[p] = on;
		}
		this.selected = [];
	}
	possible(e: Entry, p: Page) {
		return p === 'status' ? e.check : e.kind === 'web' || e.kind === 'link';
	}

	shownOn(e: Entry, p: Page) {
		return p === 'apps' ? e.apps : e.status;
	}

	/** Moves entries (the dragged one, or every selected one when it is selected) before `before`, or to the end. */
	move(id: string, to: string, before?: string) {
		const ids = this.selected.includes(id) ? [...this.selected] : [id];
		this.note(
			ids.length > 1
				? `${ids.length} entries moved to ${to}`
				: `${this.items[id]!.name} moved to ${to}`
		);
		for (const g of this.groups) g.ids = g.ids.filter((x) => !ids.includes(x));
		const g = this.groups.find((x) => x.name === to);
		if (!g) return;
		const at = before ? g.ids.indexOf(before) : -1;
		g.ids.splice(at < 0 ? g.ids.length : at, 0, ...ids);
		for (const x of ids) this.items[x]!.group = to === 'Other' ? '' : to;
		this.selected = [];
	}

	moveGroup(name: string, before: string) {
		if (name === 'Other' || name === before) return;
		this.note(`Group ${name} moved`);
		const g = this.groups.find((x) => x.name === name)!;
		const rest = this.groups.filter((x) => x.name !== name);
		const at = rest.findIndex((x) => x.name === before);
		rest.splice(at < 0 ? rest.length - 1 : at, 0, g); // "Other" stays last
		this.groups = rest;
	}

	shiftGroup(name: string, by: -1 | 1) {
		const i = this.groups.findIndex((x) => x.name === name);
		const j = i + by;
		if (j < 0 || j >= this.groups.length - 1 || name === 'Other') return;
		const g = [...this.groups];
		[g[i], g[j]] = [g[j]!, g[i]!];
		this.groups = g;
	}

	sortAZ(name: string) {
		this.note(`${name} sorted A–Z`);
		const g = this.groups.find((x) => x.name === name)!;
		g.ids = [...g.ids].sort((a, b) => byName(this.items[a]!, this.items[b]!));
	}

	addGroup() {
		let n = 1;
		while (this.groups.some((g) => g.name === `New group ${n}`)) n++;
		this.groups.splice(this.groups.length - 1, 0, { name: `New group ${n}`, ids: [] });
	}

	rename(from: string, to: string) {
		const g = this.groups.find((x) => x.name === from);
		if (!g || !to.trim() || from === 'Other') return;
		g.name = to.trim();
		for (const id of g.ids) this.items[id]!.group = g.name;
	}

	toggle(id: string, p: Page) {
		const e = this.items[id]!;
		if (!this.possible(e, p)) return;
		this.note(`${e.name} ${e[p] ? 'hidden from' : 'shown on'} the ${p} page`);
		e[p] = !e[p];
	}

	select(id: string) {
		this.selected = this.selected.includes(id)
			? this.selected.filter((x) => x !== id)
			: [...this.selected, id];
	}

	/** Live dragging (sortable.ts): the order changes while the pointer moves; one Undo for the whole drag. */
	private before: string | null = null;
	private fromGroup = '';
	start(kind: Kind, id: string) {
		this.before = this.snap();
		this.fromGroup = kind === 'item' ? this.groupOf(id).name : '';
		this.dragging = { kind, id };
	}
	over(kind: Kind, id: string, hit: Hit) {
		if (kind === 'group') return this.overGroup(id, hit);
		const ids = this.selected.includes(id) ? [...this.selected] : [id];
		const to = hit.group ?? this.groupOf(id).name;
		const g = this.groups.find((x) => x.name === to);
		if (!g || (hit.id && ids.includes(hit.id))) return;
		let before: string | undefined;
		if (hit.id) {
			const from = this.groupOf(id);
			// Within one group the side follows the direction of travel, so a swap never flips back under the pointer.
			const after = from.name === to ? g.ids.indexOf(hit.id) > g.ids.indexOf(id) : hit.after;
			const rest = g.ids.filter((x) => !ids.includes(x));
			const at = rest.indexOf(hit.id) + (after ? 1 : 0);
			before = rest[at];
		}
		const now = this.groupOf(id);
		const cur = now.ids.indexOf(id);
		if (
			now.name === to &&
			(before ? now.ids[cur + ids.length] === before : cur === now.ids.length - ids.length)
		)
			return; // already there
		for (const x of this.groups) x.ids = x.ids.filter((y) => !ids.includes(y));
		const at = before ? g.ids.indexOf(before) : g.ids.length;
		g.ids.splice(at, 0, ...ids);
		for (const x of ids) this.items[x]!.group = to === 'Other' ? '' : to;
	}
	private overGroup(name: string, hit: Hit) {
		if (name === 'Other' || !hit.id || hit.id === name) return;
		const i = this.groups.findIndex((x) => x.name === name);
		const j = this.groups.findIndex((x) => x.name === hit.id);
		if (j === this.groups.length - 1) return; // "Other" stays last
		const g = [...this.groups];
		const [moved] = g.splice(i, 1);
		g.splice(j, 0, moved!);
		this.groups = g;
	}
	end() {
		const d = this.dragging;
		if (d && this.before && this.before !== this.snap()) {
			const n = d.kind === 'item' && this.selected.includes(d.id) ? this.selected.length : 1;
			const text =
				d.kind === 'group'
					? `Group ${d.id} moved`
					: n > 1
						? `${n} entries moved`
						: this.groupOf(d.id).name === this.fromGroup
							? `${this.items[d.id]!.name} moved`
							: `${this.items[d.id]!.name} moved to ${this.groupOf(d.id).name}`;
			this.last = { text, before: this.before };
			this.said = text;
			if (n > 1) this.selected = [];
		}
		this.before = null;
		this.dragging = null;
	}
}
