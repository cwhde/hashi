<script lang="ts">
	import Tag from '../Tag.svelte';
	import { actionMeta, type Action } from '$lib/design/security';

	let { action, groups }: { action: Action; groups?: string[] } = $props();
</script>

<Tag tone={actionMeta[action].tone}
	>{actionMeta[action].label}{#if groups?.length}<span class="font-normal opacity-80"
			>· {groups.join(', ')}</span
		>{/if}</Tag
>
