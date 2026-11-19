import { execFileSync } from 'node:child_process';
import { expect, test } from 'vitest';

const run = (file: string) => {
	try {
		return {
			code: 0,
			out: execFileSync('node', ['scripts/check-spaces.mjs', file], { encoding: 'utf8' })
		};
	} catch (e) {
		const err = e as { status: number; stdout: string };
		return { code: err.status, out: err.stdout };
	}
};

// Bug 148: both shapes seen on prod ("AS6730Sunrise GmbH", "Refused by the WAFGET") are caught; the fixed ones pass.
test('finds words Svelte would render glued together', () => {
	const glued = run('scripts/testdata/glued.svelte');
	expect(glued.code).toBe(1);
	expect(glued.out).toContain('2 places');
	expect(run('scripts/testdata/spaced.svelte').code).toBe(0);
});
