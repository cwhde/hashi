<script lang="ts">
	// Mock (07 §8.1): one entry in the layout outline. Drag the row (by its grip on touch); from the keyboard, focus the
	// grip and use Alt+Up/Down. A checkbox selects several to drag or switch together. The two switches say where it
	// shows; one that is not possible is disabled with its reason. An entry hidden on the previewed page stays in place,
	// dimmed.
	import AppIcon from '#lib/components/hashi/AppIcon.svelte';
	import { Checkbox } from '#lib/components/ui/checkbox/index.ts';
	import { Activity, AppWindow, GripVertical } from '@lucide/svelte';
	import type { LayoutState, Page } from './ideaLayout.svelte.ts';

	let {
		s,
		id,
		preview,
		flash = false
	}: { s: LayoutState; id: string; preview: Page; flash?: boolean } = $props();
	const e = $derived(s.items[id]!);
	const off = $derived(!s.shownOn(e, preview));
	const sel = $derived(s.selected.includes(id));
	const kinds = { web: 'Web', link: 'Link', tcp: 'TCP', udp: 'UDP' } as const;
	const why = { apps: 'No web address', status: 'No check' } as const;
	function key(ev: KeyboardEvent) {
		if (!ev.altKey || (ev.key !== 'ArrowUp' && ev.key !== 'ArrowDown')) return;
		ev.preventDefault();
		s.shift(id, ev.key === 'ArrowUp' ? -1 : 1);
		requestAnimationFrame(() => document.getElementById('grip-' + id)?.focus());
	}
</script>

<div
	id="o-{id}"
	class="flex cursor-grab items-center gap-1.5 rounded-lg border bg-card px-1.5 py-0.5 active:cursor-grabbing {off
		? 'border-dashed opacity-50'
		: ''} {sel ? 'ring-2 ring-primary' : ''} {flash ? 'ring-2 ring-warn' : ''}"
>
	<button
		type="button"
		id="grip-{id}"
		data-grip
		onkeydown={key}
		aria-label="Move {e.name} ({kinds[e.kind]}): Alt and the arrow keys"
		class="grid size-6 shrink-0 cursor-grab touch-none place-items-center rounded-md text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring"
		><GripVertical class="size-4" /></button
	>
	<Checkbox checked={sel} onCheckedChange={() => s.select(id)} aria-label="Select {e.name}" />
	<AppIcon name={e.name} class="size-5 rounded text-[10px]" />
	<span class="min-w-0 flex-1 truncate text-md font-medium">{e.name}</span>
	<span class="hidden text-xs text-muted-foreground sm:inline">{kinds[e.kind]}</span>
	{#each [['apps', AppWindow, 'apps page'], ['status', Activity, 'status page']] as const as [p, Icon, label] (p)}
		{@const can = s.possible(e, p)}
		<button
			type="button"
			aria-pressed={e[p]}
			title={!can ? why[p] : e[p] ? `On the ${label}` : `Not on the ${label}`}
			aria-label="{e.name} on the {label}{can ? '' : `: ${why[p].toLowerCase()}`}"
			disabled={!can}
			onclick={() => s.toggle(id, p)}
			class="grid size-6 shrink-0 place-items-center rounded-md border {e[p] && can
				? 'bg-primary text-primary-foreground'
				: 'text-muted-foreground'} disabled:opacity-30"><Icon class="size-3.5" /></button
		>
	{/each}
</div>
