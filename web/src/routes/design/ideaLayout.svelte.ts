// Idea mock state for the layout editor: ONE order for both public pages. Every entry (service, DNS link, check) has a
// place in one group; each page shows the entries switched on for it and skips the rest, keeping the shared order.
import { entries, groupOrder, type Entry } from './ideas.ts';

export type Page = 'apps' | 'status';
export type Group = { name: string; ids: string[] };

const byName = (a: Entry, b: Entry) => a.name.localeCompare(b.name);

export class LayoutState {
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
	dragging = $state<{ kind: 'item' | 'group'; id: string } | null>(null);

	shownOn(e: Entry, p: Page) {
		return p === 'apps' ? e.apps : e.status;
	}

	/** Moves entries (the dragged one, or every selected one when it is selected) before `before`, or to the end. */
	move(id: string, to: string, before?: string) {
		const ids = this.selected.includes(id) ? [...this.selected] : [id];
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
		if (p === 'status' && !e.check) return;
		if (p === 'apps' && e.kind === 'udp') return;
		e[p] = !e[p];
	}

	select(id: string) {
		this.selected = this.selected.includes(id)
			? this.selected.filter((x) => x !== id)
			: [...this.selected, id];
	}

	/** Drag and drop handlers (native HTML drag; the real build would add keyboard moves and touch). */
	dragItem(id: string) {
		return {
			draggable: true,
			ondragstart: (ev: DragEvent) => {
				this.dragging = { kind: 'item', id };
				ev.dataTransfer?.setData('text/plain', id);
			},
			ondragend: () => (this.dragging = null)
		};
	}
	dropOnItem(group: string, before: string) {
		return {
			ondragover: (ev: DragEvent) => this.dragging?.kind === 'item' && ev.preventDefault(),
			ondrop: (ev: DragEvent) => {
				ev.preventDefault();
				ev.stopPropagation();
				if (this.dragging?.kind === 'item' && this.dragging.id !== before)
					this.move(this.dragging.id, group, before);
				this.dragging = null;
			}
		};
	}
	dropOnGroup(group: string) {
		return {
			ondragover: (ev: DragEvent) => this.dragging && ev.preventDefault(),
			ondrop: (ev: DragEvent) => {
				ev.preventDefault();
				if (this.dragging?.kind === 'item') this.move(this.dragging.id, group);
				else if (this.dragging?.kind === 'group') this.moveGroup(this.dragging.id, group);
				this.dragging = null;
			}
		};
	}
	dragGroup(name: string) {
		return {
			draggable: name !== 'Other',
			ondragstart: (ev: DragEvent) => {
				ev.stopPropagation();
				this.dragging = { kind: 'group', id: name };
				ev.dataTransfer?.setData('text/plain', name);
			},
			ondragend: () => (this.dragging = null)
		};
	}
}
