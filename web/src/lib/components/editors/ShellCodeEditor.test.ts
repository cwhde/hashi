/** @vitest-environment jsdom */
import { describe, expect, it } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import { EditorView } from 'codemirror';
import ShellCodeEditorTestHost from './ShellCodeEditorTestHost.svelte';

function emptyClientRects() {
	return {
		length: 0,
		item: () => null,
		[Symbol.iterator]: function* () {}
	};
}

describe('ShellCodeEditor', () => {
	it('mounts and round-trips bound value', async () => {
		if (!Range.prototype.getClientRects) {
			Range.prototype.getClientRects = emptyClientRects as () => DOMRectList;
		}
		if (!Range.prototype.getBoundingClientRect) {
			Range.prototype.getBoundingClientRect = (() => new DOMRect()) as () => DOMRect;
		}

		render(ShellCodeEditorTestHost, { props: { initialValue: 'echo "hello"\n' } });

		const editorRoot = document.querySelector('.cm-editor');
		expect(editorRoot).toBeTruthy();

		const view = EditorView.findFromDOM(editorRoot as HTMLElement);
		expect(view).toBeTruthy();
		expect(screen.getByTestId('editor-value').textContent).toBe('echo "hello"\n');

		view?.dispatch({
			changes: { from: 0, to: view.state.doc.length, insert: 'printf "updated"\n' }
		});
		await Promise.resolve();
		expect(screen.getByTestId('editor-value').textContent).toBe('printf "updated"\n');

		await fireEvent.click(screen.getByTestId('set-parent'));
		expect(view?.state.doc.toString()).toBe('#!/usr/bin/env bash\necho done\n');
	});
});
