<script lang="ts">
	import { untrack } from 'svelte';
	import Panel from './Panel.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import * as ToggleGroup from '$lib/components/ui/toggle-group';
	import { Download, Upload, Plus, X } from '@lucide/svelte';
	import { contrast } from '$lib/design/contrast';
	import {
		customVars,
		isLight,
		minContrast,
		themeProblems,
		themes,
		type CustomTheme,
		type ThemeId
	} from '$lib/theme.svelte';

	// The custom theme editor (10-ui §2): six colours, optional chart colours and roundness, previewed on this panel only
	// until saved. Saving is refused while text is unreadable; the server checks the same.
	let {
		saved,
		onsave,
		onreset,
		busy = false
	}: {
		saved: CustomTheme | null;
		onsave: (c: CustomTheme) => Promise<void>;
		onreset: (id: ThemeId) => Promise<void>;
		busy?: boolean;
	} = $props();

	// Graphite's roles (10-ui §2) as the starting point for a first custom theme.
	const start: CustomTheme = {
		canvas: '#08090b',
		surface: '#13161a',
		text: '#f2f5f7',
		muted: '#9da6b2',
		border: '#2b323b',
		accent: '#2563eb',
		roundness: 'default'
	};
	const fields = [
		['canvas', 'Canvas', 'The page behind everything'],
		['surface', 'Surface', 'Cards and panels'],
		['text', 'Text', ''],
		['muted', 'Muted text', 'Descriptions and labels'],
		['border', 'Border', ''],
		['accent', 'Accent', 'Buttons and highlights']
	] as const;

	// The draft starts as a copy of the saved theme, once; edits never touch the saved one.
	let draft = $state<CustomTheme>(untrack(() => structuredClone($state.snapshot(saved) ?? start)));
	let importError = $state('');
	let fileInput: HTMLInputElement;

	const problems = $derived(themeProblems(draft));
	const ratios = $derived(
		problems.some((p) => p.includes('must be a colour'))
			? null
			: { text: contrast(draft.text, draft.surface), muted: contrast(draft.muted, draft.surface) }
	);
	const style = $derived(
		problems.some((p) => p.includes('must be a colour'))
			? ''
			: Object.entries(customVars(draft))
					.map(([k, v]) => `${k}:${v}`)
					.join(';')
	);
	const dirty = $derived(JSON.stringify(draft) !== JSON.stringify(saved));

	function exportTheme() {
		const blob = new Blob([JSON.stringify($state.snapshot(draft), null, 2) + '\n'], {
			type: 'application/json'
		});
		const a = document.createElement('a');
		a.href = URL.createObjectURL(blob);
		a.download = 'hashi-theme.json';
		a.click();
		URL.revokeObjectURL(a.href);
	}

	async function importTheme(f: File | undefined) {
		importError = '';
		if (!f) return;
		try {
			const v = JSON.parse(await f.text()) as Partial<CustomTheme>;
			const next: CustomTheme = {
				...start,
				...Object.fromEntries(
					Object.entries(v).filter(([k]) =>
						[
							'canvas',
							'surface',
							'text',
							'muted',
							'border',
							'accent',
							'charts',
							'roundness'
						].includes(k)
					)
				)
			};
			if (!['sharp', 'default', 'round'].includes(next.roundness)) next.roundness = 'default';
			draft = next;
		} catch {
			importError = 'That file is not a Hashi theme (JSON with canvas, surface, text … colours).';
		}
		fileInput.value = '';
	}
</script>

<Panel
	title="Custom theme"
	description="Your own colours, previewed before saving."
>
	{#snippet actions()}
		<input
			bind:this={fileInput}
			type="file"
			accept="application/json,.json"
			class="hidden"
			aria-label="Theme file to import"
			onchange={(e) => importTheme(e.currentTarget.files?.[0])}
		/>
		<Button variant="ghost" size="sm" onclick={() => fileInput.click()}><Upload />Import</Button>
		<Button variant="ghost" size="sm" onclick={exportTheme}><Download />Export</Button>
	{/snippet}
	<div class="grid gap-5 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
		<div class="space-y-4">
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
				{#each fields as [k, label, hint] (k)}
					<label class="space-y-1.5">
						<span class="block text-[13px] font-medium">{label}</span>
						<span class="flex items-center gap-2"
							><input
								type="color"
								value={/^#[0-9a-f]{6}$/i.test(draft[k]) ? draft[k] : '#000000'}
								oninput={(e) => (draft[k] = e.currentTarget.value)}
								class="size-9 shrink-0 cursor-pointer rounded-md border bg-transparent"
								aria-label="{label} colour picker"
							/><Input bind:value={draft[k]} class="font-mono" aria-label="{label} colour" /></span
						>
						{#if hint}<span class="block text-[12px] text-muted-foreground">{hint}</span>{/if}
					</label>
				{/each}
			</div>
			<div class="space-y-1.5">
				<span class="block text-[13px] font-medium">Corners</span>
				<ToggleGroup.Root
					type="single"
					variant="outline"
					value={draft.roundness}
					onValueChange={(v) => v && (draft.roundness = v as CustomTheme['roundness'])}
					aria-label="Corners"
				>
					<ToggleGroup.Item value="sharp">Sharp</ToggleGroup.Item>
					<ToggleGroup.Item value="default">Default</ToggleGroup.Item>
					<ToggleGroup.Item value="round">Round</ToggleGroup.Item>
				</ToggleGroup.Root>
			</div>
			<div class="space-y-1.5">
				<span class="block text-[13px] font-medium">Chart colours</span>
				<div class="flex flex-wrap items-center gap-2">
					{#each draft.charts ?? [] as c, i (i)}
						<span class="flex items-center gap-1"
							><input
								type="color"
								value={c}
								oninput={(e) => (draft.charts![i] = e.currentTarget.value)}
								class="size-8 cursor-pointer rounded-md border bg-transparent"
								aria-label="Chart colour {i + 1}"
							/><Button
								variant="ghost"
								size="icon"
								aria-label="Remove chart colour {i + 1}"
								onclick={() => draft.charts!.splice(i, 1)}><X /></Button
							></span
						>
					{/each}
					{#if (draft.charts?.length ?? 0) < 5}<Button
							variant="ghost"
							size="sm"
							onclick={() => (draft.charts = [...(draft.charts ?? []), draft.accent])}
							><Plus />Add</Button
						>{/if}
				</div>
				{#if !draft.charts?.length}<p class="text-[12px] text-muted-foreground">
						None: charts use the accent and the status colours.
					</p>{/if}
			</div>
		</div>

		<div class="min-w-0">
			<div
				data-theme="custom"
				data-mode={problems.length === 0 && isLight(draft) ? 'light' : 'dark'}
				{style}
				class="rounded-xl border bg-background p-3 text-foreground"
				aria-label="Preview"
				role="img"
			>
				<div class="rounded-lg border bg-card p-3">
					<div class="flex items-center justify-between gap-2">
						<div class="min-w-0">
							<div class="text-[14px] font-semibold">Grafana</div>
							<div class="truncate text-[12.5px] text-muted-foreground">
								grafana.example.com · 48 ms
							</div>
						</div>
						<span class="rounded-md bg-ok/15 px-1.5 py-0.5 text-[12px] text-ok-text">up</span>
					</div>
					<div class="mt-3 h-1.5 rounded-full bg-secondary">
						<div class="h-full w-2/3 rounded-full bg-primary"></div>
					</div>
					<div class="mt-3 flex justify-end gap-2">
						<span class="rounded-md border px-2.5 py-1 text-[12.5px]">Cancel</span>
						<span
							class="rounded-md bg-primary px-2.5 py-1 text-[12.5px] font-medium text-primary-foreground"
							>Save changes</span
						>
					</div>
				</div>
			</div>
			<dl class="mt-3 grid grid-cols-[1fr_auto] gap-x-3 gap-y-1 text-[13px]">
				{#if ratios}
					<dt>Text on surface</dt>
					<dd class={ratios.text >= minContrast ? 'text-ok-text' : 'text-bad-text'}>
						{ratios.text.toFixed(1)} : 1
					</dd>
					<dt>Muted text on surface</dt>
					<dd class={ratios.muted >= minContrast ? 'text-ok-text' : 'text-bad-text'}>
						{ratios.muted.toFixed(1)} : 1
					</dd>
				{/if}
			</dl>
			{#each problems as p (p)}<p role="alert" class="mt-1 text-[12.5px] text-bad-text">
					{p}
				</p>{/each}
			{#if importError}<p role="alert" class="mt-1 text-[12.5px] text-bad-text">
					{importError}
				</p>{/if}
			<div class="mt-3 flex flex-wrap justify-end gap-2">
				{#each themes as t (t.id)}{#if t.id === 'graphite'}<Button
							variant="ghost"
							size="sm"
							disabled={busy}
							onclick={() => onreset(t.id)}>Use {t.name} again</Button
						>{/if}{/each}
				<Button
					size="sm"
					disabled={busy || problems.length > 0 || !dirty}
					onclick={() => onsave($state.snapshot(draft))}
					>{problems.length ? 'Text too faint to save' : 'Save and use theme'}</Button
				>
			</div>
		</div>
	</div>
</Panel>
