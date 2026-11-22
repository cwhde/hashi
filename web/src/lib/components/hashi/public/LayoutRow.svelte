<script lang="ts">
	// One entry in the Public pages outline (07 §8.1): grip, checkbox, icon, name, kind and the two switches, on one
	// 28 px line. The row drags (on touch by its grip, or after holding still); from the keyboard, focus the grip and use
	// Alt+Up/Down. The icon opens the icon picker (bug 149). A switch that is not possible is disabled with its reason;
	// an entry hidden from the previewed page stays in place, dimmed.
	import AppIcon from '#lib/components/hashi/AppIcon.svelte';
	import AppIconPicker from '#lib/components/hashi/AppIconPicker.svelte';
	import { Checkbox } from '#lib/components/ui/checkbox/index.ts';
	import { Activity, AppWindow, GripVertical } from '@lucide/svelte';
	import type { Schemas } from '#lib/api/client.ts';
	import type { LayoutState, Page } from '#lib/public-layout.svelte.ts';

	let {
		s,
		id,
		preview,
		flash = false,
		icon,
		oniconchange
	}: {
		s: LayoutState;
		id: string;
		preview: Page;
		flash?: boolean;
		icon?: Schemas['AppIcon'];
		oniconchange: (a: Schemas['AppIcon']) => void;
	} = $props();
	const e = $derived(s.items[id]!);
	const off = $derived(!s.shownOn(e, preview));
	const sel = $derived(s.selected.includes(id));
	const kinds: Record<string, string> = {
		web: 'Web',
		link: 'Link',
		tcp: 'TCP',
		udp: 'UDP',
		check: 'Check'
	};
	const why = { apps: 'No web address', status: 'No check' } as const;
	const pages = [
		['apps', AppWindow, 'apps page'],
		['status', Activity, 'status page']
	] as const;
	function key(ev: KeyboardEvent) {
		if (!ev.altKey || (ev.key !== 'ArrowUp' && ev.key !== 'ArrowDown')) return;
		ev.preventDefault();
		s.shift(id, ev.key === 'ArrowUp' ? -1 : 1);
		requestAnimationFrame(() => document.getElementById('grip-' + id)?.focus());
	}
</script>

<div
	id="o-{id}"
	class="flex h-7 cursor-grab items-center gap-1.5 rounded-lg border bg-card px-1 active:cursor-grabbing {off
		? 'border-dashed opacity-60'
		: ''} {sel ? 'ring-2 ring-primary' : ''} {flash ? 'ring-2 ring-warn' : ''}"
>
	<button
		type="button"
		id="grip-{id}"
		data-grip
		onkeydown={key}
		aria-label="Move {e.name}: Alt and the arrow keys"
		class="grid size-6 shrink-0 cursor-grab touch-none place-items-center rounded-md text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring"
		><GripVertical class="size-4" /></button
	>
	<Checkbox checked={sel} onCheckedChange={() => s.select(id)} aria-label="Select {e.name}" />
	{#if icon}<AppIconPicker
			app={icon}
			onchange={oniconchange}
			size="size-5 text-[10px]"
		/>{:else}<AppIcon
			name={e.name}
			icon={e.icon}
			light={e.icon_light}
			class="size-5 text-[10px]"
		/>{/if}
	<span class="min-w-0 flex-1 truncate text-md font-medium">{e.name}</span>
	<span class="hidden text-xs text-muted-foreground sm:inline">{kinds[e.type] ?? e.type}</span>
	{#each pages as [p, Icon, label] (p)}
		{@const can = s.possible(e, p)}
		<button
			type="button"
			aria-pressed={can && e[p]}
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
