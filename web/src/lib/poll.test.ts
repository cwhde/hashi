import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { every } from './poll.ts';

// A tab left open in the background polled the server around the clock (Activity every 2 s, the public pages every
// 30 s). Refreshes now wait while the tab is hidden, and one runs when it is shown again.
class FakeDocument extends EventTarget {
	hidden = false;
	show(on: boolean) {
		this.hidden = !on;
		this.dispatchEvent(new Event('visibilitychange'));
	}
}
let doc: FakeDocument;
beforeEach(() => {
	vi.useFakeTimers();
	doc = new FakeDocument();
	vi.stubGlobal('document', doc);
});
afterEach(() => {
	vi.useRealTimers();
	vi.unstubAllGlobals();
});

test('refreshes on its interval while the tab is shown', () => {
	const fn = vi.fn();
	every(1000, fn);
	vi.advanceTimersByTime(3000);
	expect(fn).toHaveBeenCalledTimes(3);
});

test('waits while the tab is hidden and refreshes once when it is shown again', () => {
	const fn = vi.fn();
	every(1000, fn);
	doc.show(false);
	vi.advanceTimersByTime(10_000);
	expect(fn).not.toHaveBeenCalled();
	doc.show(true);
	expect(fn).toHaveBeenCalledTimes(1);
});

test('stops completely once cleaned up', () => {
	const fn = vi.fn();
	const stop = every(1000, fn);
	stop();
	vi.advanceTimersByTime(5000);
	doc.show(false);
	doc.show(true);
	expect(fn).not.toHaveBeenCalled();
});
