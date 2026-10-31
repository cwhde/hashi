// IP sets (05a): API types and the small helpers the IP set pages and pickers share.
import { api, type Schemas } from '$lib/api/client';

export type IPSet = Schemas['IPSet'];
export type IPItem = Schemas['IPItem'];
export type IPSetElement = Schemas['IPSetElement'];
export type ItemType = IPItem['type'];

export const kindLabel: Record<IPSet['kind'], string> = {
	user: 'Yours',
	system: 'System',
	feed: 'Feed',
	dynamic: 'Dynamic'
};

export const lockReason: Record<IPSet['kind'], string> = {
	user: '',
	system: 'Maintained by Hashi. You can use it anywhere but not change it.',
	feed: 'Owned by the feed: updated on every fetch. Change the feed settings instead.',
	dynamic: 'Filled by the abuse engine. You can remove entries or add one with a duration.'
};

/** The item's value in its natural form (address, prefix, name, host or set id). */
export function itemValue(i: IPItem): string {
	switch (i.type) {
		case 'ip':
			return i.address;
		case 'cidr':
			return i.prefix;
		case 'dns':
			return i.fqdn;
		case 'host':
			return i.host_id;
		case 'pulse':
			return i.agent_id;
		case 'set':
			return i.set_id;
	}
}

/** Whether the text looks like an address or prefix (switches list filters to "contains"). */
export const looksLikeAddress = (q: string) =>
	/^[0-9a-f:.]+(\/\d+)?$/i.test(q.trim()) && /[.:]/.test(q);

export const addressHref = (a: string) => `/ip-sets/address?ip=${encodeURIComponent(a)}`;

export async function listSets(): Promise<IPSet[]> {
	const r = await api.GET('/api/ip-sets');
	if (r.error || !r.data) throw r.error;
	return r.data.sets;
}

/** The item editor's fields; one form serves every type (05a §2). */
export type ItemForm = {
	type: ItemType;
	value: string;
	hostId: string;
	scope: 'public' | 'internal' | 'all';
	agentId: string;
	selector: string;
	setId: string;
	note: string;
	/** yyyy-mm-dd or empty; the item expires at the end of that local day. */
	expires: string;
};

export const emptyItemForm = (): ItemForm => ({
	type: 'ip',
	value: '',
	hostId: '',
	scope: 'public',
	agentId: '',
	selector: '',
	setId: '',
	note: '',
	expires: ''
});

export function formFromElement(e: IPSetElement): ItemForm {
	const f = emptyItemForm();
	f.type = e.item.type;
	f.note = e.note;
	if (e.expires_at) {
		const d = new Date(e.expires_at);
		f.expires = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
	}
	const i = e.item;
	if (i.type === 'host') {
		f.hostId = i.host_id;
		f.scope = i.scope;
	} else if (i.type === 'pulse') {
		f.agentId = i.agent_id;
		f.scope = i.scope;
		f.selector = i.selector ?? '';
	} else if (i.type === 'set') f.setId = i.set_id;
	else f.value = itemValue(i);
	return f;
}

export function elementInput(f: ItemForm): Schemas['IPSetElementInput'] {
	const v = f.value.trim();
	const item: IPItem =
		f.type === 'ip'
			? { type: 'ip', address: v }
			: f.type === 'cidr'
				? { type: 'cidr', prefix: v }
				: f.type === 'dns'
					? { type: 'dns', fqdn: v }
					: f.type === 'host'
						? { type: 'host', host_id: f.hostId, scope: f.scope }
						: f.type === 'pulse'
							? {
									type: 'pulse',
									agent_id: f.agentId,
									scope: f.scope,
									selector: f.selector.trim() || undefined
								}
							: { type: 'set', set_id: f.setId };
	return {
		item,
		note: f.note.trim(),
		expires_at: f.expires ? new Date(f.expires + 'T23:59:59').toISOString() : null
	};
}

/** "1 item", "3 items". */
export const count = (n: number, one: string, many = one + 's') => `${n} ${n === 1 ? one : many}`;
