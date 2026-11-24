import { expect, test } from 'vitest';
import { publicSummary, stripSummary } from './monitor.ts';

// Bugs 80 and 145: say what is wrong and stop; claim nothing when nothing is listed; degraded checks count.
test('public summary', () => {
	expect(publicSummary([])).toBeNull();
	expect(publicSummary(['up', 'up'])).toEqual({ text: 'All services are working.', tone: 'ok' });
	expect(publicSummary(['down', 'up'])?.text).toBe('1 service is down.');
	expect(publicSummary(['down', 'down', 'down', 'down', 'down', 'down', 'up'])?.text).toBe(
		'6 services are down.'
	);
	expect(publicSummary(['degraded', 'up'])).toEqual({
		text: '1 service is degraded.',
		tone: 'warn'
	});
	expect(publicSummary(['down', 'down', 'degraded'])?.text).toBe(
		'2 services are down, 1 degraded.'
	);
});

// Bug 168: a paused service or one without data yet is not known to work, so the claim covers only those checked.
test('public summary with unchecked services', () => {
	expect(publicSummary(['up', 'paused'])).toEqual({
		text: 'All checked services are working.',
		tone: 'ok'
	});
	expect(publicSummary(['up', 'unknown'])?.text).toBe('All checked services are working.');
	expect(publicSummary(['down', 'paused'])?.text).toBe('1 service is down.');
	expect(publicSummary(['unknown', 'paused'])).toBeNull();
});

// Bug 87: the padding before the first checks is not counted.
test('strip summary', () => {
	expect(stripSummary(Array(60).fill('unknown'))).toBe('No checks yet');
	expect(stripSummary([...Array(58).fill('unknown'), 'up', 'down'])).toBe(
		'2 checks: 1 down, 0 degraded'
	);
	expect(stripSummary([...Array(59).fill('unknown'), 'degraded'])).toBe(
		'1 check: 0 down, 1 degraded'
	);
});
