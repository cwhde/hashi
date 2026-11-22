import { expect, test } from 'vitest';
import {
	dayDetail,
	dayName,
	duration,
	minuteDetail,
	minuteHealth,
	sinceText,
	worst,
	type Day
} from './public.ts';

const at = (hhmm: string) => {
	const [h, m] = hhmm.split(':').map(Number);
	return new Date(2026, 10, 20, h, m).toISOString();
};

test('duration', () => {
	expect(duration(10_000)).toBe('1 min');
	expect(duration(31 * 60_000)).toBe('31 min');
	expect(duration(125 * 60_000)).toBe('2 h 5 min');
	expect(duration(14 * 3_600_000)).toBe('14 h');
	expect(duration(76 * 3_600_000)).toBe('3 d 4 h');
});

test('day names are Hashi’s dates as given', () => {
	expect(dayName('2026-11-21', false)).toBe('Sat 21 Nov');
	expect(dayName('2026-11-22', true)).toBe('Today');
});

// 07 §7.1: a bar names its downtime with times and length; a slow day says slow; a day without data says so.
test('day details', () => {
	const day = (d: Partial<Day>): Day => ({ date: '2026-11-20', state: 'up', spells: [], ...d });
	expect(dayDetail(day({ uptime: 100 }), false)).toEqual({
		when: 'Fri 20 Nov',
		text: 'No downtime',
		extra: 'Uptime 100%'
	});
	expect(
		dayDetail(
			day({
				state: 'down',
				uptime: 99.93,
				spells: [
					{ state: 'degraded', start: at('10:00'), end: at('10:20') },
					{ state: 'down', start: at('19:55'), end: at('19:56') }
				]
			}),
			false
		)
	).toEqual({ when: 'Fri 20 Nov', text: 'Down 19:55–19:56 (1 min)', extra: 'Uptime 99.93%' });
	expect(
		dayDetail(
			day({
				state: 'degraded',
				spells: [{ state: 'degraded', start: at('14:10'), end: at('14:30') }]
			}),
			false
		).text
	).toBe('Slow 14:10–14:30 (20 min)');
	expect(
		dayDetail(
			day({ state: 'down', spells: [{ state: 'down', start: at('13:36') }] }),
			true,
			Date.parse(at('13:42'))
		)
	).toEqual({ when: 'Today', text: 'Down since 13:36', extra: '6 min so far' });
	expect(dayDetail(day({ uptime: 98.96 }), false)).toEqual({
		when: 'Fri 20 Nov',
		text: 'Some checks failed',
		extra: 'Uptime 98.96%'
	});
	expect(dayDetail(day({ state: 'unknown' }), false)).toEqual({
		when: 'Fri 20 Nov',
		text: 'No data'
	});
});

test('the last hour pads on the left and names each round', () => {
	const minutes = [
		{ at: at('13:40'), state: 'up' as const, latency_ms: 42 },
		{ at: at('13:41'), state: 'down' as const, latency_ms: 0 }
	];
	const h = minuteHealth(minutes);
	expect(h.length).toBe(60);
	expect(h.slice(-3)).toEqual(['unknown', 'up', 'down']);
	expect(minuteDetail(minutes, 58)).toEqual({ when: '13:40', text: 'Up', extra: '42 ms' });
	expect(minuteDetail(minutes, 59)).toEqual({ when: 'Now', text: 'Down' });
	expect(minuteDetail(minutes, 3).text).toBe('No data');
});

test('since and worst', () => {
	const now = Date.parse(at('13:42'));
	expect(sinceText('down', at('13:36'), now)).toBe('Down for 6 min');
	expect(sinceText('degraded', at('13:30'), now)).toBe('Slow for 12 min');
	expect(sinceText('unknown', at('13:30'), now)).toBe('No data yet');
	expect(worst(['up', 'degraded', 'up'])).toBe('degraded');
	expect(worst(['up', 'down', 'degraded'])).toBe('down');
	expect(worst([])).toBe('unknown');
});
