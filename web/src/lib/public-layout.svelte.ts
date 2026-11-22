// The Public pages editor's state (07 §8.1): ONE order for both public pages. Every entry has a place in one group;
// "Other" (no id) is always last. Every change saves at once as the whole order plus the switches that changed, one save
// at a time; a change made while a save runs goes out with the next one. The last change can be undone.
// It is also the drag host of `sortable.ts` (ADR 0004): the order changes while the pointer moves, one Undo and one save
// per drag, at release.
import type { Schemas } from '#lib/api/client.ts';
import type { DragHost, Hit, Kind } from '#lib/sortable.ts';

export type Layout = Schemas['PublicLayout'];
export type Entry = Schemas['PublicLayoutEntry'];
export type Save = Schemas['PublicLayoutSave'];
export type Page = 'apps' | 'status';
/** `key` is the group's identity in the editor: its id, "other", or a new group's own key until it is saved. */
export type Group = { key: string; id?: string; name: string; ids: string[] };
type Switches = Record<string, { apps: boolean; status: boolean }>;
type Snap = { groups: Group[]; sw: Switches };
/** What a save or load answers, as openapi-fetch gives it. */
export type Answer = { data?: Layout; error?: unknown; response?: { status: number } };

export const OTHER = 'Other';
export const OTHER_KEY = 'other';
const byName = (a: Entry, b: Entry) => a.name.localeCompare(b.name);
const clone = <T>(v: T): T => JSON.parse(JSON.stringify(v)) as T;

export class LayoutState implements DragHost {
	items = $state<Record<string, Entry>>({});
	groups = $state<Group[]>([]);
	selected = $state<string[]>([]);
	dragging = $state<{ kind: Kind; id: string } | null>(null);
	/** The last change, for the Undo bar. */
	last = $state<{ text: string; before: Snap } | null>(null);
	/** For screen readers: what the last change did. */
	said = $state('');
	/** A save that failed, or a name that can't be used; cleared by the next change. */
	error = $state('');
	saving = $state(false);

	private version = 0;
	/** The switches as last saved: only those that differ are sent. */
	private savedSw: Switches = {};
	private fresh = 0;
	/** Each group's id by its editor key, once the server gave one (a snapshot taken earlier may lack it). */
	private ids: Record<string, string> = {};

	constructor(
		layout: Layout,
		private io: {
			put: (body: Save) => Promise<Answer>;
			get: () => Promise<Answer>;
			/** Turns a failed answer into words. */
			explain?: (error: unknown) => string;
		}
	) {
		this.load(layout);
	}

	load(l: Layout) {
		this.version = l.version;
		const items: Record<string, Entry> = {};
		this.groups = l.groups.map((g) => {
			for (const e of g.entries) items[e.key] = e;
			return { key: g.id ?? OTHER_KEY, id: g.id, name: g.name, ids: g.entries.map((e) => e.key) };
		});
		if (!this.groups.some((g) => g.key === OTHER_KEY))
			this.groups.push({ key: OTHER_KEY, name: OTHER, ids: [] });
		this.items = items;
		this.savedSw = this.switches();
		this.selected = this.selected.filter((k) => k in items);
	}

	private switches(): Switches {
		return Object.fromEntries(
			Object.values(this.items).map((e) => [e.key, { apps: e.apps, status: e.status }])
		);
	}
	private snap(): Snap {
		return { groups: clone(this.groups), sw: this.switches() };
	}
	private same(a: Snap, b: Snap) {
		return JSON.stringify(a) === JSON.stringify(b);
	}

	/** The save request for the current order: named groups in order, Other's entries, and the switches that changed. */
	body(): Save {
		const sw = this.switches();
		return {
			version: this.version,
			groups: this.groups
				.filter((g) => g.key !== OTHER_KEY)
				.map((g) => ({ ...(g.id ? { id: g.id } : {}), name: g.name, entries: [...g.ids] })),
			other: [...(this.groups.find((g) => g.key === OTHER_KEY)?.ids ?? [])],
			switches: Object.entries(sw)
				.filter(
					([k, v]) => v.apps !== this.savedSw[k]?.apps || v.status !== this.savedSw[k]?.status
				)
				.map(([entry, v]) => ({ entry, ...v }))
		};
	}

	/** Records a change made since `before` and saves it. */
	private commit(text: string, before: Snap) {
		if (this.same(before, this.snap())) return;
		this.last = { text, before };
		this.said = text;
		this.error = '';
		void this.persist();
	}
	/** Runs a change and commits it under `text`. */
	private change(text: string, fn: () => void) {
		const before = this.snap();
		fn();
		this.commit(text, before);
	}

	private again = false;
	/** One save at a time; a change during a save is sent right after it, with the version that save returned. */
	async persist() {
		if (this.saving) {
			this.again = true;
			return;
		}
		this.saving = true;
		try {
			do {
				this.again = false;
				const body = this.body();
				// The keys of the named groups, in the order sent: the answer lists them in the same order.
				const keys = this.groups.filter((g) => g.key !== OTHER_KEY).map((g) => g.key);
				const r = await this.io.put(body);
				if (!r.data) {
					await this.failed(r);
					return;
				}
				this.saved(r.data, body, keys);
			} while (this.again);
		} finally {
			this.saving = false;
		}
	}
	private saved(l: Layout, sent: Save, keys: string[]) {
		this.version = l.version;
		for (const s of sent.switches ?? [])
			this.savedSw[s.entry] = { apps: !!s.apps, status: !!s.status };
		// New groups get their id from the answer, by place: the group may have been renamed since it was sent.
		sent.groups.forEach((g, i) => {
			const id = l.groups[i]?.id;
			if (g.id || !id) return;
			this.ids[keys[i]!] = id;
			const mine = this.group(keys[i]!);
			if (mine && !mine.id) mine.id = id;
		});
	}
	private async failed(r: Answer) {
		this.again = false;
		const stale = r.response?.status === 409;
		this.error = stale
			? 'The layout was changed elsewhere. This is the latest one; make your change again.'
			: `Not saved: ${this.io.explain?.(r.error) ?? 'the server refused it'}. This is the saved layout.`;
		this.last = null;
		const fresh = await this.io.get();
		if (fresh.data) this.load(fresh.data);
	}

	undo() {
		const l = this.last;
		if (!l) return;
		const sw = l.before.sw;
		this.groups = clone(l.before.groups).map((g) => ({ ...g, id: g.id ?? this.ids[g.key] }));
		for (const [k, v] of Object.entries(sw)) {
			const e = this.items[k];
			if (e) Object.assign(e, v);
		}
		this.last = null;
		this.said = `Undone: ${l.text}`;
		this.error = '';
		void this.persist();
	}

	group(key: string) {
		return this.groups.find((g) => g.key === key);
	}
	groupOf(id: string) {
		return this.groups.find((g) => g.ids.includes(id))!;
	}
	possible(e: Entry, p: Page) {
		return p === 'apps' ? e.can_apps : e.can_status;
	}
	shownOn(e: Entry, p: Page) {
		return p === 'apps' ? e.apps && e.can_apps : e.status && e.can_status;
	}

	/** Keyboard move (Alt+Up/Down): one place, across into the next group at a group's ends. */
	shift(id: string, by: -1 | 1) {
		const gi = this.groups.findIndex((g) => g.ids.includes(id));
		const g = this.groups[gi]!;
		const i = g.ids.indexOf(id);
		const name = this.items[id]!.name;
		if ((by < 0 && i > 0) || (by > 0 && i < g.ids.length - 1)) {
			this.change(`${name} moved ${by < 0 ? 'up' : 'down'} in ${g.name}`, () => {
				const ids = [...g.ids];
				[ids[i], ids[i + by]] = [ids[i + by]!, ids[i]!];
				g.ids = ids;
			});
			return;
		}
		const to = this.groups[gi + by];
		if (!to) return;
		this.change(`${name} moved to ${to.name}`, () => {
			g.ids = g.ids.filter((x) => x !== id);
			to.ids = by < 0 ? [...to.ids, id] : [id, ...to.ids];
		});
	}

	/** Keyboard move of a whole group; Other stays last. */
	shiftGroup(key: string, by: -1 | 1) {
		const i = this.groups.findIndex((x) => x.key === key);
		const j = i + by;
		if (key === OTHER_KEY || j < 0 || j >= this.groups.length - 1) return;
		this.change(`Group ${this.groups[i]!.name} moved`, () => {
			const g = [...this.groups];
			[g[i], g[j]] = [g[j]!, g[i]!];
			this.groups = g;
		});
	}

	/** The selection bar's Move to: every selected entry to the end of a group, in their current order. */
	moveSelected(to: string) {
		const g = this.group(to);
		if (!g || !this.selected.length) return;
		const order = this.groups.flatMap((x) => x.ids).filter((k) => this.selected.includes(k));
		const text =
			order.length > 1
				? `${order.length} entries moved to ${g.name}`
				: `${this.items[order[0]!]!.name} moved to ${g.name}`;
		this.change(text, () => {
			for (const x of this.groups) x.ids = x.ids.filter((k) => !order.includes(k));
			g.ids = [...g.ids, ...order];
		});
		this.selected = [];
	}

	/** The selection bar's Show or hide: every selected entry where it is possible. */
	setSelected(p: Page, on: boolean) {
		const list = this.selected.map((k) => this.items[k]!).filter((e) => this.possible(e, p));
		const page = p === 'apps' ? 'apps page' : 'status page';
		this.change(
			`${list.length} ${list.length === 1 ? 'entry' : 'entries'} ${on ? 'shown on' : 'hidden from'} the ${page}`,
			() => {
				for (const e of list) e[p] = on;
			}
		);
		this.selected = [];
	}

	toggle(id: string, p: Page) {
		const e = this.items[id]!;
		if (!this.possible(e, p)) return;
		const page = p === 'apps' ? 'apps page' : 'status page';
		this.change(`${e.name} ${e[p] ? 'hidden from' : 'shown on'} the ${page}`, () => (e[p] = !e[p]));
	}

	select(id: string) {
		this.selected = this.selected.includes(id)
			? this.selected.filter((x) => x !== id)
			: [...this.selected, id];
	}

	sortAZ(key: string) {
		const g = this.group(key)!;
		this.change(`${g.name} sorted A–Z`, () => {
			g.ids = [...g.ids].sort((a, b) => byName(this.items[a]!, this.items[b]!));
		});
	}

	/** A new, empty group before Other; returns its key so the page can start renaming it. */
	addGroup(): string {
		let n = 1;
		while (this.groups.some((g) => g.name.toLowerCase() === `new group ${n}`)) n++;
		const key = `new-${++this.fresh}`;
		this.change(`Group New group ${n} added`, () => {
			this.groups.splice(this.groups.length - 1, 0, { key, name: `New group ${n}`, ids: [] });
		});
		return key;
	}

	/** Only an empty group can go (its entries would have nowhere to be). */
	deleteGroup(key: string) {
		const g = this.group(key);
		if (!g || key === OTHER_KEY || g.ids.length) return;
		this.change(`Group ${g.name} deleted`, () => {
			this.groups = this.groups.filter((x) => x.key !== key);
		});
	}

	/** Why a name can't be used, or '' when it can. */
	badName(key: string, name: string): string {
		const n = name.trim();
		if (!n) return 'A group needs a name.';
		if (n.toLowerCase() === OTHER.toLowerCase())
			return '“Other” is kept for entries without a group.';
		if ([...n].length > 60) return 'At most 60 characters.';
		if (this.groups.some((g) => g.key !== key && g.name.toLowerCase() === n.toLowerCase()))
			return `There is already a group called ${n}.`;
		return '';
	}

	rename(key: string, name: string): boolean {
		const g = this.group(key);
		if (!g || key === OTHER_KEY) return false;
		const bad = this.badName(key, name);
		if (bad) {
			this.error = bad;
			return false;
		}
		const n = name.trim();
		if (n === g.name) return true;
		this.change(`${g.name} renamed to ${n}`, () => (g.name = n));
		return true;
	}

	// Live dragging: the order changes while the pointer moves; one Undo and one save for the whole drag.
	private before: Snap | null = null;
	private fromGroup = '';
	start(kind: Kind, id: string) {
		this.before = this.snap();
		this.fromGroup = kind === 'item' ? this.groupOf(id).key : '';
		this.dragging = { kind, id };
	}
	over(kind: Kind, id: string, hit: Hit) {
		if (kind === 'group') return this.overGroup(id, hit);
		const ids = this.selected.includes(id)
			? this.groups.flatMap((g) => g.ids).filter((k) => this.selected.includes(k))
			: [id];
		const to = hit.group ?? this.groupOf(id).key;
		const g = this.group(to);
		if (!g || (hit.id && ids.includes(hit.id))) return;
		let before: string | undefined;
		if (hit.id) {
			const from = this.groupOf(id);
			// Within one group the side follows the direction of travel, so a swap never flips back under the pointer.
			const after = from.key === to ? g.ids.indexOf(hit.id) > g.ids.indexOf(id) : hit.after;
			const rest = g.ids.filter((x) => !ids.includes(x));
			before = rest[rest.indexOf(hit.id) + (after ? 1 : 0)];
		}
		const now = this.groupOf(ids[0]!);
		const cur = now.ids.indexOf(ids[0]!);
		const block = now.ids.slice(cur, cur + ids.length).join() === ids.join();
		if (
			now.key === to &&
			block &&
			(before ? now.ids[cur + ids.length] === before : cur === now.ids.length - ids.length)
		)
			return; // already there
		for (const x of this.groups) x.ids = x.ids.filter((y) => !ids.includes(y));
		const at = before ? g.ids.indexOf(before) : g.ids.length;
		g.ids.splice(at, 0, ...ids);
	}
	private overGroup(key: string, hit: Hit) {
		if (key === OTHER_KEY || !hit.id || hit.id === key) return;
		const i = this.groups.findIndex((x) => x.key === key);
		const j = this.groups.findIndex((x) => x.key === hit.id);
		if (i < 0 || j < 0 || j === this.groups.length - 1) return; // Other stays last
		const g = [...this.groups];
		const [moved] = g.splice(i, 1);
		g.splice(j, 0, moved!);
		this.groups = g;
	}
	end() {
		const d = this.dragging;
		const before = this.before;
		this.before = null;
		this.dragging = null;
		if (!d || !before || this.same(before, this.snap())) return;
		const n = d.kind === 'item' && this.selected.includes(d.id) ? this.selected.length : 1;
		const name = d.kind === 'group' ? this.group(d.id)!.name : this.items[d.id]!.name;
		const into = d.kind === 'item' ? this.groupOf(d.id) : null;
		const text =
			d.kind === 'group'
				? `Group ${name} moved`
				: n > 1
					? `${n} entries moved`
					: into!.key === this.fromGroup
						? `${name} moved`
						: `${name} moved to ${into!.name}`;
		if (n > 1) this.selected = [];
		this.commit(text, before);
	}
}
