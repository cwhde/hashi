<script lang="ts">
	import ShellCodeEditor from './ShellCodeEditor.svelte';

	let { initialValue = '' }: { initialValue?: string } = $props();
	const getInitialValue = () => initialValue;
	let value = $state(getInitialValue());

	function setFromParent() {
		value = '#!/usr/bin/env bash\necho done\n';
	}
</script>

<ShellCodeEditor bind:value />
<pre data-testid="editor-value">{value}</pre>
<button type="button" data-testid="set-parent" onclick={setFromParent}>Set parent value</button>
