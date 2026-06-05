import { describe, expect, it } from 'vitest';
import { filterTimelineEvents, parseSecuritySubjectQuery } from './subject-tools';

describe('security subject tools', () => {
	it('parses supported subject search inputs', () => {
		expect(parseSecuritySubjectQuery('203.0.113.10')).toEqual({ type: 'ip', value: '203.0.113.10' });
		expect(parseSecuritySubjectQuery('203.0.113.0/24')).toEqual({
			type: 'cidr',
			value: '203.0.113.0/24'
		});
		expect(parseSecuritySubjectQuery('64500')).toEqual({ type: 'asn', value: 'AS64500' });
		expect(parseSecuritySubjectQuery('de')).toEqual({ type: 'country', value: 'DE' });
		expect(parseSecuritySubjectQuery('us-ca')).toEqual({ type: 'region', value: 'US-CA' });
		expect(parseSecuritySubjectQuery('failed challenge')).toEqual({
			type: 'text',
			value: 'failed challenge'
		});
	});

	it('filters timeline events by type and resource', () => {
		const events = [
			{ id: '1', occurredAtUtc: '2026-06-04T00:00:00Z', eventType: 'manual_block', resourceId: 'a' },
			{ id: '2', occurredAtUtc: '2026-06-04T00:01:00Z', eventType: 'challenge', resourceId: 'a' },
			{ id: '3', occurredAtUtc: '2026-06-04T00:02:00Z', eventType: 'manual_block', resourceId: 'b' }
		];

		expect(filterTimelineEvents(events, 'manual_block', 'a').map((event) => event.id)).toEqual(['1']);
		expect(filterTimelineEvents(events, 'manual_block', '').map((event) => event.id)).toEqual([
			'1',
			'3'
		]);
		expect(filterTimelineEvents(events, '', 'a').map((event) => event.id)).toEqual(['1', '2']);
	});
});
