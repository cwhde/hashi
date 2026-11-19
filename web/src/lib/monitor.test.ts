import { expect, test } from 'vitest';
import { publicSummary } from './monitor.ts';

// Bugs 80 and 145: say what is wrong and stop; claim nothing when nothing is listed; slow checks count.
test('public summary', () => {
	expect(publicSummary([])).toBeNull();
	expect(publicSummary(['up', 'up'])).toEqual({ text: 'All services are working.', tone: 'ok' });
	expect(publicSummary(['down', 'up'])?.text).toBe('1 service is down.');
	expect(publicSummary(['down', 'down', 'down', 'down', 'down', 'down', 'up'])?.text).toBe(
		'6 services are down.'
	);
	expect(publicSummary(['degraded', 'up'])).toEqual({ text: '1 service is slow.', tone: 'warn' });
	expect(publicSummary(['down', 'down', 'degraded'])?.text).toBe('2 services are down, 1 slow.');
});
