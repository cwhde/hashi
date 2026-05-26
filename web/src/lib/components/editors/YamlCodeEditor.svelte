<script lang="ts">
	import { onMount } from 'svelte';
	import { EditorView, basicSetup } from 'codemirror';
	import { yaml } from '@codemirror/lang-yaml';
	import { oneDark } from '@codemirror/theme-one-dark';

	let {
		value = $bindable(''),
		minHeight = '18rem'
	}: {
		value?: string;
		minHeight?: string;
	} = $props();

	let host = $state<HTMLDivElement | null>(null);
	let view: EditorView | null = null;

	onMount(() => {
		if (!host) return;

		view = new EditorView({
			parent: host,
			extensions: [
				basicSetup,
				yaml(),
				oneDark,
				EditorView.updateListener.of((update) => {
					if (update.docChanged) {
						value = update.state.doc.toString();
					}
				})
			],
			doc: value
		});

		return () => {
			view?.destroy();
			view = null;
		};
	});

	$effect(() => {
		if (!view) return;
		const current = view.state.doc.toString();
		if (current !== value) {
			view.dispatch({
				changes: { from: 0, to: current.length, insert: value }
			});
		}
	});
</script>

<div
	bind:this={host}
	class="overflow-hidden rounded-md border border-border bg-hashi-bg-dark font-mono text-[11px]"
	style:min-height={minHeight}
></div>
