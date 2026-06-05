export type ParsedSecuritySubjectQuery = {
	type: 'ip' | 'cidr' | 'asn' | 'country' | 'region' | 'text';
	value: string;
};

export type SecurityTimelineEvent = {
	eventType?: string | null;
	resourceId?: string | null;
	occurredAtUtc: string;
};

const ipv4Pattern = /^(25[0-5]|2[0-4]\d|1?\d?\d)(\.(25[0-5]|2[0-4]\d|1?\d?\d)){3}$/;

export function parseSecuritySubjectQuery(input: string): ParsedSecuritySubjectQuery {
	const value = input.trim();
	if (value.includes('/')) return { type: 'cidr', value };
	if (ipv4Pattern.test(value) || value.includes(':')) return { type: 'ip', value };
	if (/^(as)?\d+$/i.test(value)) {
		const number = value.replace(/^as/i, '');
		return { type: 'asn', value: `AS${number}` };
	}
	if (/^[a-z]{2}$/i.test(value)) return { type: 'country', value: value.toUpperCase() };
	if (/^[a-z]{2}-[a-z0-9-]+$/i.test(value)) return { type: 'region', value: value.toUpperCase() };
	return { type: 'text', value };
}

export function filterTimelineEvents<T extends SecurityTimelineEvent>(
	events: T[],
	eventType: string,
	resourceId: string
): T[] {
	return events.filter((event) => {
		const typeMatches = !eventType || event.eventType === eventType;
		const resourceMatches = !resourceId || event.resourceId === resourceId;
		return typeMatches && resourceMatches;
	});
}
