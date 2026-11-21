import { expect, test, vi } from 'vitest';

const get = vi.fn();
vi.mock('#lib/api/client.ts', () => ({ api: { GET: get } }));

// 13.11: the layout and the overview both ask for the appearance as a page loads; they share one request. A later
// call reads again, so a change saved since is seen.
test('callers share a read under way, not a finished one', async () => {
	const { theme } = await import('./theme.svelte.ts');
	let answer: (v: unknown) => void = () => {};
	get.mockImplementation(() => new Promise((r) => (answer = r)));
	const a = theme.load();
	const b = theme.load();
	answer({ data: { theme: 'graphite' } });
	expect(await a).toBe(await b);
	expect(get).toHaveBeenCalledTimes(1);
	get.mockResolvedValue({ data: { theme: 'linen' } });
	await theme.load();
	expect(get).toHaveBeenCalledTimes(2);
});
