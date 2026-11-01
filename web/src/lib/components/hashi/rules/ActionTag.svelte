<script lang="ts">
	import Tag from '../Tag.svelte';
	import { actionMeta } from '$lib/rules';

	let { action, groups }: { action: string; groups?: string[] } = $props();
	const meta = $derived(actionMeta[action as keyof typeof actionMeta] ?? actionMeta.deny);
</script>

<Tag tone={meta.tone}
	>{meta.label}{#if groups?.length}<span class="font-normal opacity-80">· {groups.join(', ')}</span
		>{/if}</Tag
>
