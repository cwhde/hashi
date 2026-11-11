<script lang="ts">
	import { onMount } from 'svelte';
	import { EditorState } from '@codemirror/state';
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
		StreamLanguage,
		syntaxHighlighting,
		bracketMatching
	} from '@codemirror/language';
	import { shell } from '@codemirror/legacy-modes/mode/shell';
	import { tags } from '@lezer/highlight';

	// The script editor (09 §1, CodeMirror). Colours come from the semantic tokens, so it follows every theme.
	let {
		value = $bindable(''),
		label,
		class: className = ''
	}: { value?: string; label: string; class?: string } = $props();

	let host: HTMLDivElement;
	let view: EditorView | null = null;

	const theme = EditorView.theme({
		'&': { backgroundColor: 'transparent', color: 'var(--color-foreground)', fontSize: '12.5px' },
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
		'.cm-cursor': { borderLeftColor: 'var(--color-foreground)' }
	});
	const colours = HighlightStyle.define([
		{ tag: [tags.keyword, tags.controlKeyword], color: 'var(--color-info-text)' },
		{ tag: [tags.string, tags.special(tags.string)], color: 'var(--color-ok-text)' },
		{
			tag: [tags.variableName, tags.definition(tags.variableName)],
			color: 'var(--color-warn-text)'
		},
		{ tag: [tags.number, tags.bool], color: 'var(--color-warn-text)' },
		{
			tag: [tags.comment, tags.lineComment, tags.meta],
			color: 'var(--color-muted-foreground)',
			fontStyle: 'italic'
		},
		{ tag: [tags.operator, tags.punctuation], color: 'var(--color-muted-foreground)' }
	]);

	onMount(() => {
		view = new EditorView({
			parent: host,
			state: EditorState.create({
				doc: value,
				extensions: [
					lineNumbers(),
					history(),
					drawSelection(),
					bracketMatching(),
					highlightActiveLine(),
					StreamLanguage.define(shell),
					syntaxHighlighting(colours),
					theme,
					keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
					EditorState.tabSize.of(4),
					EditorView.contentAttributes.of({ 'aria-label': label }),
					EditorView.updateListener.of((u) => {
						if (u.docChanged) value = u.state.doc.toString();
					})
				]
			})
		});
		return () => view?.destroy();
	});

	// A value set from outside (another script picked, reset) replaces the document.
	$effect(() => {
		const v = value;
		if (view && v !== view.state.doc.toString()) {
			view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: v } });
		}
	});
</script>

<div bind:this={host} class="min-h-56 overflow-hidden {className}" data-testid="shell-editor"></div>
