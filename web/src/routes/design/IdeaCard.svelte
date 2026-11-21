<script lang="ts">
	// Idea mock: one entry on the layout board, drawn like its card on the apps page. Two small switches say where it
	// shows; an entry hidden on the page being previewed is dimmed, not removed, so it keeps its place in the order.
	import { Checkbox } from '#lib/components/ui/checkbox/index.ts';
	import { AppWindow, Activity, GripVertical } from '@lucide/svelte';
	import type { LayoutState, Page } from './ideaLayout.svelte.ts';

	let {
		s,
		id,
		group,
		preview,
		compact = false
	}: { s: LayoutState; id: string; group: string; preview: Page; compact?: boolean } = $props();
	const e = $derived(s.items[id]!);
	const off = $derived(!s.shownOn(e, preview));
	const sel = $derived(s.selected.includes(id));
</script>

<div
	{...s.dragItem(id)}
	{...s.dropOnItem(group, id)}
	class="group/card flex cursor-grab items-center gap-2 rounded-xl border bg-card active:cursor-grabbing {compact
		? 'px-2 py-1.5'
		: 'p-2.5'} {off ? 'border-dashed opacity-45' : ''} {sel ? 'ring-2 ring-primary' : ''} {s
		.dragging?.id === id
		? 'opacity-30'
		: ''}"
>
	<GripVertical class="size-4 shrink-0 text-muted-foreground" />
	<Checkbox checked={sel} onCheckedChange={() => s.select(id)} aria-label="Select {e.name}" />
	<span
		class="grid shrink-0 place-items-center rounded-lg font-semibold text-white {compact
			? 'size-6 text-xs'
			: 'size-8 text-sm'}"
		style="background:hsl({e.hue} 45% 42%)">{e.letter}</span
	>
	<span class="min-w-0 flex-1 truncate text-md font-medium">{e.name}</span>
	<span class="flex shrink-0 gap-1">
		<button
			type="button"
			title={e.kind === 'udp'
				? 'No page to open (UDP)'
				: e.apps
					? 'On the apps page'
					: 'Not on the apps page'}
			disabled={e.kind === 'udp'}
			onclick={() => s.toggle(id, 'apps')}
			class="grid size-6 place-items-center rounded-md border {e.apps
				? 'bg-primary text-primary-foreground'
				: 'text-muted-foreground'} disabled:opacity-30"><AppWindow class="size-3.5" /></button
		>
		<button
			type="button"
			title={!e.check
				? 'No uptime check'
				: e.status
					? 'On the status page'
					: 'Not on the status page'}
			disabled={!e.check}
			onclick={() => s.toggle(id, 'status')}
			class="grid size-6 place-items-center rounded-md border {e.status
				? 'bg-primary text-primary-foreground'
				: 'text-muted-foreground'} disabled:opacity-30"><Activity class="size-3.5" /></button
		>
	</span>
</div>
