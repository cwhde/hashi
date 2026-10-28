// Helpers shared by setup step 3 and /dns (03-dns.md §3–4).
import type { Schemas } from '$lib/api/client';

export type RRSet = Schemas['DNSRRSet'];
export type ImportRow = Schemas['DNSImportRow'];
export type Decision = Schemas['DNSDecision'];

/** Name + type identify an RRSet within the zone. */
export const rrsetKey = (r: Pick<RRSet, 'name' | 'type'>) => `${r.name} ${r.type}`;

/** TTL as shown in tables: an absent TTL inherits the zone default and says so. */
export const ttlText = (r: RRSet) => (r.ttl == null ? `zone · ${r.effective_ttl}` : String(r.ttl));

/**
 * Decisions the table may offer for a row. The server's allowed_decisions already cover NS/SOA, change protection
 * and types the provider cannot manage; a record Hashi already owns (other than an import) cannot be pruned here.
 */
export function offeredDecisions(row: ImportRow): Decision[] {
	const owned = row.rrset.origin !== 'unmanaged' && row.rrset.origin !== 'imported';
	return row.allowed_decisions.filter((d) => !(owned && d === 'prune'));
}

/**
 * Why a row offers only "leave", in one short sentence for the import table. The server makes a row leave-only for
 * exactly three reasons: NS/SOA (invariant 2), the provider's change protection, or a type the provider cannot manage.
 */
export function leaveOnlyReason(r: RRSet): string {
	return `Hashi leaves ${r.type} records alone.`;
}

/** The decision per row as saved: stored ones, "leave" for new rows (so a saved table has no new rows left). */
export const initialDecisions = (rows: ImportRow[]): Record<string, Decision> =>
	Object.fromEntries(rows.map((r) => [rrsetKey(r.rrset), r.decision ?? 'leave']));

/** The request body for saving: one decision per row, in table order. */
export const decisionBody = (rows: ImportRow[], decisions: Record<string, Decision>) => ({
	decisions: rows.map((r) => ({
		name: r.rrset.name,
		type: r.rrset.type,
		decision: decisions[rrsetKey(r.rrset)] ?? 'leave'
	}))
});
