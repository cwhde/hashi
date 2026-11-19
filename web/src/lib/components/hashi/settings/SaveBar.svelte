<script lang="ts">
	// The end of a settings form: what the last save did, and Save, enabled once something changed (10c §5.1).
	import { Button } from '#lib/components/ui/button/index.ts';
	import { cn } from '#lib/utils.ts';
	import type { Saver } from '#lib/settings/saver.svelte.ts';

	let {
		saver,
		dirty,
		onsave,
		label = 'Save'
	}: { saver: Saver; dirty: boolean; onsave: () => void; label?: string } = $props();
</script>

<div class="flex items-center justify-end gap-3">
	{#if saver.note}<p
			role={saver.note.ok ? 'status' : 'alert'}
			class={cn('text-md', saver.note.ok ? 'text-ok-text' : 'text-bad-text')}
		>
			{saver.note.msg}
		</p>{/if}
	<Button size="sm" onclick={onsave} disabled={saver.busy || !dirty}>{label}</Button>
</div>
