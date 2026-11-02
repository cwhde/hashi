<script lang="ts">
	import Mono from '$lib/components/hashi/Mono.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import * as Select from '$lib/components/ui/select';
	import * as Sheet from '$lib/components/ui/sheet';
	import { api, problemText, type Schemas } from '$lib/api/client';

	// Feeds (06 §8): a blocklist becomes an IP set of kind feed. Adding one downloads it once; it blocks nothing until it
	// is used, e.g. in the firewall's global block.
	let { open = $bindable(false), onadded }: { open?: boolean; onadded?: (setId: string) => void } =
		$props();

	type Format = Schemas['FeedFormat'];
	let catalog = $state<Schemas['RecommendedFeed'][]>([]);
	let custom = $state({
		url: '',
		format: 'lines' as Format,
		column: 1,
		prefix: 0,
		field: '',
		http: false
	});
	let slug = $state('');
	let preview = $state<Schemas['FeedPreview'] | null>(null);
	let error = $state('');
	let busy = $state('');

	$effect(() => {
		if (!open) return;
		preview = null;
		error = '';
		api.GET('/api/feeds/recommended').then((r) => (catalog = r.data ?? []));
	});

	const formats: Record<Format, string> = {
		lines: 'One address or range per line',
		csv: 'Columns (CSV or tab separated)',
		json_array: 'JSON list of addresses',
		json_object: 'JSON object with a list field',
		json_lines: 'JSON object per line'
	};
	const source = $derived<Schemas['FeedSource']>({
		url: custom.url.trim(),
		format: custom.format,
		allow_http: custom.http,
		options:
			custom.format === 'csv'
				? {
						separator: 'tab',
						column: custom.column,
						prefix_column: custom.prefix || undefined
					}
				: custom.format === 'json_object' || custom.format === 'json_lines'
					? { field: custom.field }
					: {}
	});

	async function add(input: Schemas['FeedInput'], key: string) {
		busy = key;
		error = '';
		const r = await api.POST('/api/feeds', { body: input });
		busy = '';
		if (r.error || !r.data) {
			error = problemText(r.error);
			return;
		}
		if (r.data.run.outcome === 'failed')
			error = `Added, but the first download failed: ${r.data.run.error}`;
		onadded?.(r.data.set_id);
	}
	const addRecommended = (e: Schemas['RecommendedFeed']) =>
		add(
			{
				name: e.name,
				slug: `feed-${e.key}`,
				description: e.description,
				catalog_key: e.key,
				url: e.url,
				format: e.format,
				options: e.options
			},
			e.key
		);
	async function previewCustom() {
		busy = 'preview';
		error = '';
		const r = await api.POST('/api/feeds/preview', { body: source });
		busy = '';
		if (r.error || !r.data) error = problemText(r.error);
		else preview = r.data;
	}
</script>

<Sheet.Root bind:open>
	<Sheet.Content class="w-full sm:max-w-xl">
		<Sheet.Header>
			<Sheet.Title>Add blocklist</Sheet.Title>
			<Sheet.Description
				>A blocklist becomes an IP set that refreshes itself. It blocks nothing until you use it,
				for example in Firewall → global block.</Sheet.Description
			>
		</Sheet.Header>
		<div class="space-y-5 overflow-y-auto px-4 pb-4">
			<ul class="space-y-2">
				{#each catalog as e (e.key)}
					<li class="flex items-start gap-3 rounded-xl border p-3 text-[13px]">
						<span class="min-w-0 flex-1">
							<span class="flex flex-wrap items-center gap-2 font-medium"
								><span>{e.name}</span><Tag tone={e.risk === 'low' ? 'neutral' : 'warn'}
									>{e.risk} false-positive risk</Tag
								></span
							>
							<span class="block text-muted-foreground">{e.description}</span>
						</span>
						{#if e.set_id}<Tag tone="ok">added</Tag>{:else}<Button
								size="sm"
								variant="outline"
								disabled={!!busy}
								onclick={() => addRecommended(e)}>{busy === e.key ? 'Adding…' : 'Add'}</Button
							>{/if}
					</li>
				{/each}
			</ul>

			<details class="rounded-xl border p-3 text-[13px]">
				<summary class="cursor-pointer font-medium">Another list by address</summary>
				<div class="mt-3 space-y-3">
					<div class="space-y-1.5">
						<Label for="fu">Address</Label>
						<Input id="fu" bind:value={custom.url} placeholder="https://…" class="font-mono" />
					</div>
					<div class="space-y-1.5">
						<Label for="ff">Format</Label>
						<Select.Root type="single" bind:value={custom.format}>
							<Select.Trigger id="ff" class="w-full">{formats[custom.format]}</Select.Trigger>
							<Select.Content>
								{#each Object.entries(formats) as [v, l] (v)}<Select.Item value={v}>{l}</Select.Item
									>{/each}
							</Select.Content>
						</Select.Root>
					</div>
					{#if custom.format === 'csv'}
						<div class="grid grid-cols-2 gap-3">
							<div class="space-y-1.5">
								<Label for="fc">Address column</Label><Input
									id="fc"
									type="number"
									bind:value={custom.column}
								/>
							</div>
							<div class="space-y-1.5">
								<Label for="fp">Prefix-length column (0: none)</Label><Input
									id="fp"
									type="number"
									bind:value={custom.prefix}
								/>
							</div>
						</div>
					{:else if custom.format === 'json_object' || custom.format === 'json_lines'}
						<div class="space-y-1.5">
							<Label for="fj">Field with the addresses</Label><Input
								id="fj"
								bind:value={custom.field}
							/>
						</div>
					{/if}
					<label class="flex items-center gap-2"
						><Switch bind:checked={custom.http} />Allow plain HTTP (not encrypted)</label
					>
					<Button
						size="sm"
						variant="outline"
						disabled={!custom.url || !!busy}
						onclick={previewCustom}>{busy === 'preview' ? 'Downloading…' : 'Preview'}</Button
					>
					{#if preview}
						<div class="space-y-2 rounded-lg bg-surface-2 p-3">
							<p>
								{preview.count.toLocaleString()} entries, {preview.rejected} not taken, {preview.ignored}
								comment lines.
							</p>
							{#if preview.sample.length}<p class="text-muted-foreground">
									First: <Mono>{preview.sample.slice(0, 5).join(', ')}</Mono>
								</p>{/if}
							{#each preview.rejects.slice(0, 5) as r, i (i)}<p class="text-muted-foreground">
									<Mono>{r.text}</Mono>: {r.reason}
								</p>{/each}
							<div class="flex gap-2 pt-1">
								<Input
									bind:value={slug}
									placeholder="feed-name"
									class="font-mono"
									aria-label="Set name"
								/>
								<Button
									disabled={!slug || !!busy}
									onclick={() => add({ ...source, name: slug, slug }, 'custom')}>Add</Button
								>
							</div>
						</div>
					{/if}
				</div>
			</details>
			{#if error}<p role="alert" class="text-[13px] text-bad-text">{error}</p>{/if}
		</div>
	</Sheet.Content>
</Sheet.Root>
