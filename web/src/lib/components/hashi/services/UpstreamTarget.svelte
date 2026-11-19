<script lang="ts">
	// What a service sends its traffic to (bug 17): an address, a Pulse agent (its internal address, followed) or an
	// IP set (every address in it). One choice, then only that choice's field.
	import { onMount, type Snippet } from 'svelte';
	import { Input } from '#lib/components/ui/input/index.ts';
	import * as Select from '#lib/components/ui/select/index.ts';
	import * as ToggleGroup from '#lib/components/ui/toggle-group/index.ts';
	import IpSetPicker from '#lib/components/hashi/ipsets/IpSetPicker.svelte';
	import { pulseAgents } from '#lib/pulseAgents.svelte.ts';
	import { targetNames } from '#lib/targetNames.svelte.ts';
	import type { Upstream } from '#lib/services.ts';

	// port: the upstream's port box, shown next to the address, agent or set it belongs to (bug 111).
	let {
		value = $bindable(),
		id = 'upstream',
		port
	}: { value: Upstream; id?: string; port?: Snippet } = $props();
	onMount(() => void targetNames.load());

	// Bug 68: two pasted lines would join into one value. Keep the first address and say what happened.
	let pasteNote = $state('');
	function paste(e: ClipboardEvent) {
		const parts = (e.clipboardData?.getData('text') ?? '').split(/[\s,]+/).filter(Boolean);
		pasteNote = '';
		if (parts.length < 2) return;
		e.preventDefault();
		value.address = parts[0] ?? '';
		pasteNote = `Kept ${parts[0]}. For more than one address, put them in an IP set.`;
	}
</script>

<div class="space-y-2">
	<ToggleGroup.Root
		type="single"
		variant="outline"
		value={value.mode}
		onValueChange={(v) => v && (value.mode = v as Upstream['mode'])}
		aria-label="Upstream kind"
	>
		<ToggleGroup.Item value="address">Address</ToggleGroup.Item>
		<ToggleGroup.Item value="agent" disabled={!pulseAgents.list.length}
			>Pulse agent</ToggleGroup.Item
		>
		<ToggleGroup.Item value="set">IP set</ToggleGroup.Item>
	</ToggleGroup.Root>
	<div class={port ? 'grid grid-cols-[minmax(0,1fr)_6rem] gap-2' : ''}>
		{#if value.mode === 'address'}
			<Input
				{id}
				bind:value={value.address}
				oninput={() => (pasteNote = '')}
				onpaste={paste}
				aria-label="Address"
				placeholder="192.0.2.20"
				required
			/>
		{:else if value.mode === 'agent'}
			<Select.Root type="single" bind:value={value.agentId}>
				<Select.Trigger {id} aria-label="Pulse agent" class="w-full"
					>{targetNames.agentName(value.agentId) ?? 'Choose an agent'}</Select.Trigger
				>
				<Select.Content>
					{#each pulseAgents.list as a (a.id)}<Select.Item value={a.id}
							>{a.name}{a.selected_ip ? ` · ${a.selected_ip}` : ''}</Select.Item
						>{/each}
				</Select.Content>
			</Select.Root>
		{:else}
			<IpSetPicker bind:value={value.setId} sets={targetNames.sets} addressOnly {id} />
		{/if}
		{@render port?.()}
	</div>
	{#if pasteNote}<p role="status" class="text-sm text-warn-text">{pasteNote}</p>{/if}
</div>
