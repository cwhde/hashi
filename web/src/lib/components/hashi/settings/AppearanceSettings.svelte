<script lang="ts">
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	// Settings → Appearance (09 §2): theme, your own theme, comfort, the Overview's widgets. A choice here applies and
	// saves at once (bug 39); only the custom theme editor has a Save, because it is a form.
	import { onMount } from 'svelte';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import CustomThemeEditor from '#lib/components/hashi/CustomThemeEditor.svelte';
	import { Switch } from '#lib/components/ui/switch/index.ts';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { withReauth } from '#lib/auth/reauth.ts';
	import { cn } from '#lib/utils.ts';
	import { theme, themes, customVars, type CustomTheme, type ThemeId } from '#lib/theme.svelte.ts';
	import { Saver } from '#lib/settings/saver.svelte.ts';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { ArrowDown, ArrowUp } from '@lucide/svelte';
	import { arrange, widgetNames, type Widget } from '#lib/overview.ts';

	let a = $state<Schemas['AppearanceSettings'] | null>(null);
	let error = $state('');
	const saver = new Saver();
	onMount(async () => {
		const r = await api.GET('/api/settings/appearance');
		if (!r.data) error = problemText(r.error);
		else a = r.data;
	});
	const saveCustom = async (c: CustomTheme) => {
		await saver.run(async () => {
			const cur = await api.GET('/api/settings/appearance');
			if (!cur.data) throw cur.error;
			a = await withReauth(() =>
				api.PUT('/api/settings/appearance', {
					body: { ...cur.data, theme: 'custom', custom_theme: c }
				})
			);
			theme.apply(a);
		}, 'Saved. Your theme is in use here, on the public pages and in alert mails.');
	};
	const useBuiltIn = async (id: ThemeId) => {
		await saver.run(async () => {
			await theme.save(id);
			a = { ...a!, theme: id };
		}, 'Switched back. Your custom theme stays saved.');
	};
	const solid = (on: boolean) => saver.run(() => theme.saveSolid(on), '');

	// The Overview's widgets (bug 31): on or off and their order, saved at once like the other choices here.
	const widgets = $derived(arrange(a?.overview_widgets));
	const saveWidgets = (next: Widget[]) =>
		saver.run(async () => {
			const cur = await api.GET('/api/settings/appearance');
			if (!cur.data) throw cur.error;
			a = await withReauth(() =>
				api.PUT('/api/settings/appearance', { body: { ...cur.data, overview_widgets: next } })
			);
		}, '');
	const toggle = (i: number, visible: boolean) =>
		saveWidgets(widgets.map((w, j) => (j === i ? { ...w, visible } : w)));
	function move(i: number, by: -1 | 1) {
		const next = [...widgets];
		[next[i], next[i + by]] = [next[i + by]!, next[i]!];
		return saveWidgets(next);
	}
</script>

{#snippet card(id: ThemeId, name: string, style: string, mode?: string)}
	<button
		onclick={() => void theme.save(id)}
		aria-pressed={theme.current === id}
		class={cn('rounded-xl border p-1.5 text-left', theme.current === id && 'ring-2 ring-ring')}
	>
		<div
			data-theme={id}
			data-mode={mode}
			{style}
			class="flex h-20 gap-1.5 rounded-lg border bg-background p-1.5"
		>
			<div class="w-5 rounded-md bg-sidebar"></div>
			<div class="flex flex-1 flex-col justify-between rounded-md border bg-card p-1.5">
				<div class="h-1.5 w-3/4 rounded-full bg-muted-foreground/40"></div>
				<div class="h-2.5 w-8 self-end rounded bg-primary"></div>
			</div>
		</div>
		<div class="px-1 pt-1.5 text-md font-medium">{name}</div>
	</button>
{/snippet}

{#if error}<p role="alert" class="text-md text-bad-text">{error}</p>
{:else if a}
	<Panel title="Theme">
		<div class="grid grid-cols-2 gap-3 md:grid-cols-4">
			{#each themes as t (t.id)}{@render card(t.id, t.name, '', t.mode)}{/each}
			{#if theme.custom}{@render card(
					'custom',
					'Custom',
					Object.entries(customVars(theme.custom))
						.map(([k, v]) => `${k}:${v}`)
						.join(';')
				)}{/if}
		</div>
	</Panel>
	<CustomThemeEditor
		saved={a.custom_theme ?? null}
		onsave={saveCustom}
		onreset={useBuiltIn}
		busy={saver.busy}
	/>
	<Panel title="Comfort">
		<label class="flex items-center justify-between gap-3 text-md"
			>Reduce transparency<Switch
				checked={theme.solid}
				onCheckedChange={(v) => void solid(v)}
				disabled={saver.busy}
			/></label
		>
	</Panel>
	<Panel title="Overview" flush>
		<ul class="divide-y" aria-label="Overview widgets">
			{#each widgets as w, i (w.id)}
				<li class="flex items-center gap-2 px-4 py-2 text-md">
					<span class="min-w-0 flex-1 truncate">{widgetNames[w.id]}</span>
					<Button
						variant="ghost"
						size="icon-sm"
						disabled={saver.busy || i === 0}
						onclick={() => void move(i, -1)}
						aria-label="Move {widgetNames[w.id]} up"><ArrowUp /></Button
					>
					<Button
						variant="ghost"
						size="icon-sm"
						disabled={saver.busy || i === widgets.length - 1}
						onclick={() => void move(i, 1)}
						aria-label="Move {widgetNames[w.id]} down"><ArrowDown /></Button
					>
					<Switch
						checked={w.visible}
						onCheckedChange={(v) => void toggle(i, v)}
						disabled={saver.busy}
						aria-label="Show {widgetNames[w.id]}"
					/>
				</li>
			{/each}
		</ul>
	</Panel>
	{#if saver.note}<p
			role={saver.note.ok ? 'status' : 'alert'}
			class={cn('text-md', saver.note.ok ? 'text-ok-text' : 'text-bad-text')}
		>
			{saver.note.msg}
		</p>{/if}
{:else}<Skeleton class="h-40 rounded-2xl" />
{/if}
