// A line diff for plan previews (09 §1: "shows target hosts and diff before applying").

export type DiffLine = { kind: 'same' | 'add' | 'del'; text: string };

const lines = (s: string) => (s === '' ? [] : s.replace(/\n$/, '').split('\n'));

/** Lines of before and after, aligned by their longest common subsequence. */
export function lineDiff(before: string, after: string): DiffLine[] {
	const a = lines(before);
	const b = lines(after);
	const w = b.length + 1;
	// lcs[i * w + j]: common lines of a[i:] and b[j:]
	const lcs = new Uint32Array((a.length + 1) * w);
	const at = (i: number, j: number) => lcs[i * w + j] ?? 0;
	for (let i = a.length - 1; i >= 0; i--)
		for (let j = b.length - 1; j >= 0; j--)
			lcs[i * w + j] = a[i] === b[j] ? at(i + 1, j + 1) + 1 : Math.max(at(i + 1, j), at(i, j + 1));
	const out: DiffLine[] = [];
	let i = 0;
	let j = 0;
	while (i < a.length || j < b.length) {
		const x = a[i];
		const y = b[j];
		if (x !== undefined && y !== undefined && x === y) {
			out.push({ kind: 'same', text: x });
			i++;
			j++;
		} else if (x !== undefined && (y === undefined || at(i + 1, j) >= at(i, j + 1))) {
			out.push({ kind: 'del', text: x });
			i++;
		} else if (y !== undefined) {
			out.push({ kind: 'add', text: y });
			j++;
		}
	}
	return out;
}
