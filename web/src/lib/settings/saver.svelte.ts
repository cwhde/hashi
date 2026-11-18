import { problemText } from '#lib/api/client.ts';
import { ceremonyText } from '#lib/auth/webauthn.ts';
import type { Draft } from '#lib/unsaved.svelte.ts';

/** One form's save: busy while it runs, then one line saying what happened or why nothing changed (10c §5). */
export class Saver {
	busy = $state(false);
	note = $state<{ ok: boolean; msg: string } | null>(null);

	async run(fn: () => Promise<unknown>, done: string, draft?: Draft): Promise<boolean> {
		this.busy = true;
		this.note = null;
		try {
			await fn();
			draft?.mark();
			if (done) this.note = { ok: true, msg: done };
			return true;
		} catch (e) {
			this.note = { ok: false, msg: e instanceof Error ? ceremonyText(e) : problemText(e) };
			return false;
		} finally {
			this.busy = false;
		}
	}
}

/** Splits a textarea of addresses or names into a list. */
export const lines = (s: string) =>
	s
		.split(/[\s,]+/)
		.map((x) => x.trim())
		.filter(Boolean);
