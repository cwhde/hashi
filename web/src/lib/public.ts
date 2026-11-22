// What the public status pages say (07 §7.1, §7.2): the words for a bar of the 90-day strip or the last hour, how long
// a state has lasted, and the worst state of a group. Times are the visitor's local time; days are Hashi's (the server
// cuts them in its own time zone and sends the date).
import type { Schemas } from '#lib/api/client.ts';
import type { Health } from '#lib/design/mock.ts';

export type Day = Schemas['PublicDay'];
export type Minute = Schemas['PublicMinute'];
export type Spell = Schemas['PublicSpell'];
export type Detail = { when: string; text: string; extra?: string };

const MIN = 60_000;

/** "1 min", "2 h 5 min", "3 d 4 h": the two largest parts, never "0 min". */
export function duration(ms: number): string {
	const m = Math.max(1, Math.round(ms / MIN));
	const d = Math.floor(m / 1440);
	const h = Math.floor((m % 1440) / 60);
	const r = m % 60;
	if (d) return h ? `${d} d ${h} h` : `${d} d`;
	if (h) return r ? `${h} h ${r} min` : `${h} h`;
	return `${r} min`;
}

export const clock = (iso: string) =>
	new Date(iso).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' });

/** "20 Nov, 09:05". */
const stamp = (iso: string) =>
	new Date(iso).toLocaleString('en-GB', {
		day: 'numeric',
		month: 'short',
		hour: '2-digit',
		minute: '2-digit'
	});

/** A moment on a public page: the time today, else the date too. */
export function when(iso: string, now = Date.now()): string {
	return new Date(iso).toDateString() === new Date(now).toDateString() ? clock(iso) : stamp(iso);
}

/** What a strip says to a screen reader: how many bars hold data, and which of them were down or slow. */
export function stripText(samples: string[], unit: 'day' | 'check'): string {
	const real = samples.filter((s) => s !== 'unknown');
	if (!real.length) return 'No data';
	const down = real.filter((s) => s === 'down').length;
	const slow = real.filter((s) => s === 'degraded').length;
	const what = `${real.length} ${unit}${real.length === 1 ? '' : 's'}`;
	const bad = [down && `${down} down`, slow && `${slow} slow`].filter(Boolean);
	return bad.length ? `${what}: ${bad.join(', ')}` : `${what}, all up`;
}

/** A day's name on the strip: "Today" or "Sat 21 Nov". The date is Hashi's day, so it is shown as given. */
export function dayName(date: string, today: boolean): string {
	if (today) return 'Today';
	const [y, m, d] = date.split('-').map(Number);
	return new Date(Date.UTC(y!, m! - 1, d!)).toLocaleDateString('en-GB', {
		weekday: 'short',
		day: 'numeric',
		month: 'short',
		timeZone: 'UTC'
	});
}

const word = { down: 'Down', degraded: 'Slow' } as const;

function spellText(s: Spell, now: number): { text: string; extra?: string } {
	if (!s.end) {
		return {
			text: `${word[s.state]} since ${clock(s.start)}`,
			extra: `${duration(now - Date.parse(s.start))} so far`
		};
	}
	const len = duration(Date.parse(s.end) - Date.parse(s.start));
	return { text: `${word[s.state]} ${clock(s.start)}–${clock(s.end)} (${len})` };
}

const pct = (v: number) => `${v >= 99.995 ? '100' : v.toFixed(2)}%`;

/** What one bar of the 90-day strip says: the day, its worst spell with times, then the day's uptime. */
export function dayDetail(day: Day, today: boolean, now = Date.now()): Detail {
	const when = dayName(day.date, today);
	if (day.state === 'unknown') return { when, text: 'No data' };
	const uptime = day.uptime === undefined ? undefined : `Uptime ${pct(day.uptime)}`;
	const worst = day.spells.filter((s) => s.state === 'down');
	const list = worst.length ? worst : day.spells;
	const first = list[0];
	// Single failed checks that never added up to an outage still lower the day's uptime: say so rather than "No
	// downtime" next to 98.96 %.
	if (!first)
		return {
			when,
			text: day.uptime !== undefined && day.uptime < 99.995 ? 'Some checks failed' : 'No downtime',
			extra: uptime
		};
	const t = spellText(first, now);
	const more = list.length > 1 ? ` and ${list.length - 1} more` : '';
	return { when, text: t.text + more, extra: t.extra ?? uptime };
}

/** The last hour strip, one bar per round, padded on the left like the admin strip. */
export function minuteHealth(minutes: Minute[], slots = 60): Health[] {
	const s = minutes.slice(-slots).map((m) => m.state as Health);
	return [...Array<Health>(slots - s.length).fill('unknown'), ...s];
}

/** What one bar of the last hour says; `i` counts the padded strip of `slots` bars. */
export function minuteDetail(minutes: Minute[], i: number, slots = 60): Detail {
	const m = minutes.slice(-slots)[i - (slots - Math.min(slots, minutes.length))];
	if (!m) return { when: 'Earlier', text: 'No data' };
	const when = i === slots - 1 ? 'Now' : clock(m.at);
	if (m.state === 'down') return { when, text: 'Down' };
	return {
		when,
		text: m.state === 'degraded' ? 'Slow' : 'Up',
		extra: `${m.latency_ms.toLocaleString('en-GB')} ms`
	};
}

/** The service page's first tile: the state as the label, how long as the value ("Up for" / "17 h 41 min"). */
export function sinceTile(state: string, since: string, now = Date.now()): [string, string] {
	const words: Record<string, string> = {
		up: 'Up for',
		down: 'Down for',
		degraded: 'Slow for',
		paused: 'Paused for'
	};
	const w = words[state];
	if (!w) return ['State', 'No data yet'];
	return [w, duration(now - Date.parse(since))];
}

const words: Record<string, string> = {
	up: 'Up',
	degraded: 'Slow',
	down: 'Down',
	paused: 'Paused'
};

/** A state in the visitors' words, the same on every public page. */
export const stateWord = (state: string) => words[state] ?? 'No data';

const rank: Record<string, number> = { down: 0, degraded: 1, unknown: 2, paused: 3, up: 4 };

/** A group's dot: its worst entry. */
export function worst(states: string[]): Health {
	let w: Health = 'up';
	for (const s of states) if ((rank[s] ?? 2) < rank[w]!) w = s as Health;
	return states.length ? w : 'unknown';
}

/** A downtime on the service page: start, end and length; "ongoing" while it lasts. */
export function downtimeRow(s: Spell, now = Date.now()) {
	return {
		start: stamp(s.start),
		end: s.end ? clock(s.end) : 'ongoing',
		length: duration((s.end ? Date.parse(s.end) : now) - Date.parse(s.start))
	};
}
