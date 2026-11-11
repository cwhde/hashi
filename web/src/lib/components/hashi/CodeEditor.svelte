<script lang="ts">
	import { onMount } from 'svelte';
	import { EditorState, type Extension } from '@codemirror/state';
	import {
		EditorView,
		keymap,
		lineNumbers,
		highlightActiveLine,
		drawSelection
	} from '@codemirror/view';
	import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
	import {
		HighlightStyle,
		syntaxHighlighting,
		indentOnInput,
		bracketMatching
	} from '@codemirror/language';
	import { yaml } from '@codemirror/lang-yaml';
	import { linter, lintGutter, setDiagnostics, type Diagnostic } from '@codemirror/lint';
	import { tags } from '@lezer/highlight';

	// CodeMirror 6 (01 §2, 10-ui §4), themed from the semantic tokens so it follows all four themes. One problem at a
	// time is shown at its line, as the server's strict check reports it.
	let {
		value = $bindable(''),
		label,
		readonly = false,
		problem = null,
		class: className = ''
	}: {
		value?: string;
		label: string;
		readonly?: boolean;
		problem?: { line: number; message: string } | null;
		class?: string;
	} = $props();

	let host: HTMLDivElement;
	let view: EditorView | null = null;

	const theme = EditorView.theme({
		'&': {
			backgroundColor: 'transparent',
			color: 'var(--color-foreground)',
			fontSize: '12.5px'
		},
		'&.cm-focused': { outline: 'none' },
		'.cm-scroller': { fontFamily: 'var(--font-mono)', lineHeight: '1.6' },
		'.cm-content': { padding: '12px 0', caretColor: 'var(--color-foreground)' },
		'.cm-gutters': {
			backgroundColor: 'transparent',
			color: 'var(--color-muted-foreground)',
			border: 'none'
		},
		'.cm-activeLine, .cm-activeLineGutter': {
			backgroundColor: 'color-mix(in oklab, var(--color-foreground) 5%, transparent)'
		},
		'.cm-selectionBackground, &.cm-focused .cm-selectionBackground, ::selection': {
			backgroundColor: 'color-mix(in oklab, var(--color-primary) 30%, transparent) !important'
		},
		'.cm-cursor': { borderLeftColor: 'var(--color-foreground)' },
		'.cm-lintRange-error': {
			backgroundImage: 'none',
			textDecoration: 'underline wavy var(--color-bad)'
		},
		'.cm-lint-marker-error': { content: 'none' },
		'.cm-tooltip': {
			backgroundColor: 'var(--color-popover)',
			color: 'var(--color-popover-foreground)',
			border: '1px solid var(--color-border)',
			borderRadius: '10px'
		}
	});
	const colours = HighlightStyle.define([
		{ tag: [tags.propertyName, tags.definition(tags.propertyName)], color: 'var(--color-primary)' },
		{ tag: [tags.string, tags.special(tags.string)], color: 'var(--color-ok-text)' },
		{ tag: [tags.number, tags.bool, tags.null], color: 'var(--color-warn-text)' },
		{
			tag: [tags.comment, tags.lineComment],
			color: 'var(--color-muted-foreground)',
			fontStyle: 'italic'
		},
		{ tag: [tags.punctuation, tags.separator], color: 'var(--color-muted-foreground)' }
	]);

	function diagnostics(): Diagnostic[] {
		if (!view || !problem) return [];
		const doc = view.state.doc;
		const n = problem.line > 0 && problem.line <= doc.lines ? problem.line : 1;
		const line = doc.line(n);
		return [{ from: line.from, to: line.to, severity: 'error', message: problem.message }];
	}

	onMount(() => {
		const exts: Extension[] = [
			lineNumbers(),
			lintGutter(),
			history(),
			drawSelection(),
			indentOnInput(),
			bracketMatching(),
			highlightActiveLine(),
			yaml(),
			syntaxHighlighting(colours),
			theme,
			keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
			linter(() => diagnostics(), { delay: 0 }),
			EditorState.tabSize.of(2),
			EditorState.readOnly.of(readonly),
			EditorView.contentAttributes.of({ 'aria-label': label }),
			EditorView.updateListener.of((u) => {
				if (u.docChanged) value = u.state.doc.toString();
			})
		];
		view = new EditorView({
			parent: host,
			state: EditorState.create({ doc: value, extensions: exts })
		});
		return () => view?.destroy();
	});

	// A value set from outside (load, reset) replaces the document.
	$effect(() => {
		const v = value;
		if (view && v !== view.state.doc.toString()) {
			view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: v } });
		}
	});
	$effect(() => {
		void problem;
		if (view) view.dispatch(setDiagnostics(view.state, diagnostics()));
	});
</script>

<div bind:this={host} class="min-h-64 overflow-hidden {className}" data-testid="code-editor"></div>
