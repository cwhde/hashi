import { describe, expect, it } from 'vitest';
import { lineDiff } from './diff';

const show = (b: string, a: string) =>
	lineDiff(b, a)
		.map((l) => ({ same: ' ', add: '+', del: '-' })[l.kind] + l.text)
		.join('\n');

describe('lineDiff', () => {
	it('keeps common lines and marks the changed one', () => {
		expect(show('#!/bin/sh\necho a\nexit 0\n', '#!/bin/sh\necho b\nexit 0\n')).toBe(
			' #!/bin/sh\n-echo a\n+echo b\n exit 0'
		);
	});
	it('a new file is all additions, a removed one all deletions', () => {
		expect(show('', 'x\ny')).toBe('+x\n+y');
		expect(show('x\ny\n', '')).toBe('-x\n-y');
	});
	it('equal texts have no changes', () => {
		expect(lineDiff('a\nb', 'a\nb').every((l) => l.kind === 'same')).toBe(true);
	});
});
