import { describe, expect, test, vi } from 'vitest';
import {
	LayoutState,
	OTHER_KEY,
	type Answer,
	type Entry,
	type Layout,
	type Save
} from './public-layout.svelte.ts';

const entry = (key: string, name: string, more: Partial<Entry> = {}): Entry => ({
	key,
	name,
	type: 'web',
	apps: true,
	status: true,
	can_apps: true,
	can_status: true,
	slug: name.toLowerCase(),
	...more
});

function layout(): Layout {
	return {
		version: 3,
		groups: [
			{
				id: 'g1',
				name: 'Media',
				entries: [entry('a', 'Arr'), entry('b', 'Bazarr'), entry('c', 'Cinema')]
			},
			{ id: 'g2', name: 'Home', entries: [entry('d', 'Door'), entry('e', 'Energy')] },
			{
				name: 'Other',
				entries: [entry('f', 'Files', { type: 'tcp', can_apps: false, apps: false })]
			}
		]
	};
}

/** A server that answers every save with the next version and the entries it kept, and records what it got. Like
 * the real one it refuses a group id it doesn't know, and `leaves` names entries a save takes off the layout. */
function server(start: Layout = layout(), leaves: (body: Save) => string[] = () => []) {
	const got: Save[] = [];
	let version = 3;
	let fresh = 0;
	const all = Object.fromEntries(start.groups.flatMap((g) => g.entries).map((e) => [e.key, e]));
	let known = new Set(start.groups.flatMap((g) => (g.id ? [g.id] : [])));
	const put = vi.fn(async (body: Save): Promise<Answer> => {
		got.push(structuredClone(body));
		const bad = body.groups.find((g) => g.id && !known.has(g.id));
		if (bad) return { error: { detail: `unknown group "${bad.id}"` }, response: { status: 422 } };
		version++;
		const gone = leaves(body);
		const list = (keys: string[]) => keys.filter((k) => !gone.includes(k)).map((k) => all[k]!);
		const groups = body.groups.map((g) => ({
			id: g.id ?? `new${++fresh}`,
			name: g.name,
			entries: list(g.entries)
		}));
		known = new Set(groups.map((g) => g.id));
		return { data: { version, groups: [...groups, { name: 'Other', entries: list(body.other) }] } };
	});
	const get = vi.fn(async (): Promise<Answer> => ({ data: layout() }));
	return { got, put, get };
}

const order = (s: LayoutState) => s.groups.map((g) => `${g.name}:${g.ids.join('')}`).join(' ');
const settle = () => new Promise((r) => setTimeout(r, 0));

describe('dragging', () => {
	test('within a group the side follows the direction of travel', () => {
		const s = new LayoutState(layout(), server());
		s.start('item', 'a');
		// Moving down onto Cinema's upper half still lands after it: the pointer is travelling down.
		s.over('item', 'a', { id: 'c', group: 'g1', after: false });
		expect(order(s)).toBe('Media:bca Home:de Other:f');
		// Moving up onto Bazarr's lower half lands before it: the pointer is travelling up.
		s.over('item', 'a', { id: 'b', group: 'g1', after: true });
		expect(order(s)).toBe('Media:abc Home:de Other:f');
	});

	test('across groups the half of the target decides, and an empty space joins the end', () => {
		const s = new LayoutState(layout(), server());
		s.start('item', 'b');
		s.over('item', 'b', { id: 'e', group: 'g2', after: false });
		expect(order(s)).toBe('Media:ac Home:dbe Other:f');
		s.over('item', 'b', { id: 'e', group: 'g2', after: true });
		expect(order(s)).toBe('Media:ac Home:deb Other:f');
		s.over('item', 'b', { group: OTHER_KEY, after: true });
		expect(order(s)).toBe('Media:ac Home:de Other:fb');
	});

	test('checked entries move together, in their order, and the drag is one change', async () => {
		const io = server();
		const s = new LayoutState(layout(), io);
		s.select('c');
		s.select('a');
		s.start('item', 'c');
		s.over('item', 'c', { id: 'd', group: 'g2', after: false });
		expect(order(s)).toBe('Media:b Home:acde Other:f');
		s.over('item', 'c', { id: 'e', group: 'g2', after: true });
		expect(order(s)).toBe('Media:b Home:deac Other:f');
		s.end();
		await settle();
		expect(s.last?.text).toBe('2 entries moved');
		expect(s.selected).toEqual([]);
		expect(io.put).toHaveBeenCalledTimes(1);
		expect(io.got[0]).toMatchObject({
			version: 3,
			groups: [
				{ id: 'g1', name: 'Media', entries: ['b'] },
				{ id: 'g2', name: 'Home', entries: ['d', 'e', 'a', 'c'] }
			],
			other: ['f'],
			switches: []
		});
	});

	test('a drag that ends where it started saves nothing', async () => {
		const io = server();
		const s = new LayoutState(layout(), io);
		s.start('item', 'a');
		s.over('item', 'a', { id: 'b', group: 'g1', after: true });
		s.over('item', 'a', { id: 'b', group: 'g1', after: false });
		expect(order(s)).toBe('Media:abc Home:de Other:f');
		s.end();
		await settle();
		expect(io.put).not.toHaveBeenCalled();
		expect(s.last).toBeNull();
	});

	test('groups move by group, and Other stays last', () => {
		const s = new LayoutState(layout(), server());
		s.start('group', 'g2');
		s.over('group', 'g2', { id: 'g1', after: false });
		expect(s.groups.map((g) => g.name)).toEqual(['Home', 'Media', 'Other']);
		s.over('group', 'g2', { id: OTHER_KEY, after: true });
		expect(s.groups.map((g) => g.name)).toEqual(['Home', 'Media', 'Other']);
		s.start('group', OTHER_KEY);
		s.over('group', OTHER_KEY, { id: 'g2', after: false });
		expect(s.groups.at(-1)!.name).toBe('Other');
	});
});

describe('keyboard and the selection bar', () => {
	test('Alt+arrows move one place, then across into the next group', () => {
		const s = new LayoutState(layout(), server());
		s.shift('b', -1);
		expect(order(s)).toBe('Media:bac Home:de Other:f');
		expect(s.said).toBe('Bazarr moved up in Media');
		s.shift('b', -1);
		expect(order(s)).toBe('Media:bac Home:de Other:f'); // nothing above the first group
		s.shift('c', 1);
		expect(order(s)).toBe('Media:ba Home:cde Other:f');
		expect(s.said).toBe('Cinema moved to Home');
	});

	test('Move to keeps the layout order of the checked entries', () => {
		const s = new LayoutState(layout(), server());
		s.select('e');
		s.select('a');
		s.moveSelected(OTHER_KEY);
		expect(order(s)).toBe('Media:bc Home:d Other:fae');
		expect(s.last?.text).toBe('2 entries moved to Other');
	});

	test('Show or hide only touches entries where it is possible, and sends only those switches', async () => {
		const io = server();
		const s = new LayoutState(layout(), io);
		s.select('a');
		s.select('f'); // a TCP entry has no web address
		s.setSelected('apps', false);
		await settle();
		expect(s.items.a!.apps).toBe(false);
		expect(s.items.f!.apps).toBe(false);
		expect(io.got[0]!.switches).toEqual([{ entry: 'a', apps: false, status: true }]);
		expect(s.last?.text).toBe('1 entry hidden from the apps page');
	});
});

describe('saving and undo', () => {
	test('a change during a save goes out next, with the version that save returned', async () => {
		const io = server();
		let release!: () => void;
		const first = new Promise<void>((r) => (release = r));
		const put = io.put.getMockImplementation()!;
		io.put.mockImplementationOnce(async (b) => {
			await first;
			return put(b);
		});
		const s = new LayoutState(layout(), io);
		s.shift('a', 1);
		s.shift('d', 1);
		s.toggle('b', 'status');
		release();
		await vi.waitFor(() => expect(io.put).toHaveBeenCalledTimes(2));
		await settle();
		expect(io.got[0]!.version).toBe(3);
		expect(io.got[1]).toMatchObject({ version: 4, switches: [{ entry: 'b', status: false }] });
		expect(s.saving).toBe(false);
	});

	test('undo puts back the order and the switches, and saves that', async () => {
		const io = server();
		const s = new LayoutState(layout(), io);
		s.toggle('a', 'apps');
		await settle();
		s.undo();
		await settle();
		expect(s.items.a!.apps).toBe(true);
		expect(s.said).toBe('Undone: Arr hidden from the apps page');
		expect(io.got[1]!.switches).toEqual([{ entry: 'a', apps: true, status: true }]);
		s.shift('a', 1);
		s.undo();
		expect(order(s)).toBe('Media:abc Home:de Other:f');
	});

	test('a layout changed elsewhere reloads the saved one and says so', async () => {
		const io = server();
		io.put.mockResolvedValueOnce({ error: {}, response: { status: 409 } });
		const s = new LayoutState(layout(), io);
		s.shift('a', 1);
		await vi.waitFor(() => expect(io.get).toHaveBeenCalled());
		await settle();
		expect(order(s)).toBe('Media:abc Home:de Other:f');
		expect(s.error).toMatch(/changed elsewhere/);
		expect(s.last).toBeNull();
	});

	test('a group renamed before its first save answers still gets its id, and keeps it through undo', async () => {
		const io = server();
		let release!: () => void;
		const first = new Promise<void>((r) => (release = r));
		const put = io.put.getMockImplementation()!;
		io.put.mockImplementationOnce(async (b) => {
			await first;
			return put(b);
		});
		const s = new LayoutState(layout(), io);
		const key = s.addGroup();
		s.rename(key, 'Games');
		release();
		await vi.waitFor(() => expect(io.put).toHaveBeenCalledTimes(2));
		await settle();
		expect(s.group(key)!.id).toBe('new1');
		expect(io.got[1]!.groups[2]).toEqual({ id: 'new1', name: 'Games', entries: [] });
		s.undo(); // back to "New group 1", which the server knows by its id now
		await settle();
		expect(io.got[2]!.groups[2]).toEqual({ id: 'new1', name: 'New group 1', entries: [] });
	});

	test('a new group gets its id from the answer; names follow the server rules', async () => {
		const io = server();
		const s = new LayoutState(layout(), io);
		const key = s.addGroup();
		await settle();
		expect(s.group(key)).toMatchObject({ name: 'New group 1', id: 'new1' });
		expect(s.rename(key, ' other ')).toBe(false);
		expect(s.error).toMatch(/kept for entries without a group/);
		expect(s.rename(key, 'media')).toBe(false);
		expect(s.rename(key, 'Games')).toBe(true);
		s.deleteGroup('g1'); // not empty
		expect(s.group('g1')).toBeDefined();
		s.deleteGroup(key);
		expect(s.group(key)).toBeUndefined();
	});
});

describe('saving', () => {
	test('a save that cannot reach the server says so and shows the saved layout (bug 174)', async () => {
		const io = server();
		io.put.mockRejectedValueOnce(new TypeError('Failed to fetch'));
		const s = new LayoutState(layout(), io);
		s.addGroup();
		await settle();
		expect(s.error).toMatch(/^Not saved/);
		expect(s.last).toBeNull();
		expect(order(s)).toBe('Media:abc Home:de Other:f');
	});

	test('when the saved layout cannot be read either, the last saved order comes back (bug 174)', async () => {
		const io = server();
		io.put.mockRejectedValueOnce(new TypeError('Failed to fetch'));
		io.get.mockRejectedValueOnce(new TypeError('Failed to fetch'));
		const s = new LayoutState(layout(), io);
		s.shift('a', 1);
		await settle();
		expect(s.error).toMatch(/^Not saved/);
		expect(order(s)).toBe('Media:abc Home:de Other:f');
	});

	test('Undo brings back a deleted group as a new one (bug 163)', async () => {
		const l = layout();
		l.groups.splice(2, 0, { id: 'g3', name: 'Tools', entries: [] });
		const io = server(l);
		const s = new LayoutState(l, io);
		s.deleteGroup('g3');
		await settle();
		s.undo();
		await settle();
		expect(s.error).toBe('');
		expect(io.got.at(-1)!.groups.map((g) => g.id ?? g.name)).toEqual(['g1', 'g2', 'Tools']);
		expect(s.group('g3')?.id).toMatch(/^new/);
	});

	test('an entry a save takes off the layout leaves the outline, without an Undo (bug 164)', async () => {
		const io = server(layout(), (b) =>
			(b.switches ?? []).filter((x) => x.entry === 'e' && !x.apps && !x.status).map((x) => x.entry)
		);
		const s = new LayoutState(layout(), io);
		s.toggle('e', 'status');
		await settle();
		s.toggle('e', 'apps');
		await settle();
		expect(order(s)).toBe('Media:abc Home:d Other:f');
		expect(s.items.e).toBeUndefined();
		expect(s.last).toBeNull();
		expect(s.said).toBe('Energy hidden from the apps page. It is off both public pages now.');
	});
});
