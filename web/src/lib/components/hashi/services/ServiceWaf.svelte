<script lang="ts">
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	import { onMount } from 'svelte';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import HelpTip from '#lib/components/hashi/HelpTip.svelte';
	import Mono from '#lib/components/hashi/Mono.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import * as Select from '#lib/components/ui/select/index.ts';
	import { Plus, Trash2, X } from '@lucide/svelte';
	import WafRulePicker from './WafRulePicker.svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { Draft } from '#lib/unsaved.svelte.ts';

	// WAF per path (06 §9): headers only or off. It runs in forward auth, which never sees request bodies, so the mock's
	// "enabled" mode is not offered. The draft saves with the service page's one Save (10c §5, bug 65): the page reads
	// `draft.dirty` and calls `save()`.
	let { serviceId }: { serviceId: string } = $props();

	// A path row as the form edits it: mode '' follows the service.
	type Path = {
		id?: string;
		match: 'prefix' | 'exact';
		path: string;
		mode: string;
		exclusions: number[];
	};
	let view = $state<Schemas['ServiceWAFView'] | null>(null);
	let mode = $state(''); // '' = the default
	let exclusions = $state<number[]>([]);
	let rules = $state<Schemas['WAFRule'][]>([]);
	const ruleById = $derived(new Map(rules.map((r) => [r.id, r])));
	let paths = $state<Path[]>([]);
	let error = $state('');
	export const draft = new Draft(() => [mode, exclusions, paths]);

	function take(v: Schemas['ServiceWAFView']) {
		view = v;
		mode = v.mode ?? '';
		exclusions = [...(v.exclusions ?? [])];
		paths = (v.paths ?? []).map((p) => ({
			id: p.id,
			match: p.match,
			path: p.path,
			mode: p.mode ?? '',
			exclusions: p.exclusions ?? []
		}));
		draft.mark();
	}
	onMount(async () => {
		const [r, list] = await Promise.all([
			api.GET('/api/services/{id}/waf', { params: { path: { id: serviceId } } }),
			api.GET('/api/waf/rules')
		]);
		rules = list.data?.rules ?? []; // without names the numbers still show and save
		if (r.error || !r.data) error = problemText(r.error);
		else take(r.data);
	});

	const label = (m: string | undefined, inherit: string) =>
		!m ? inherit : m === 'headers_only' ? 'headers only' : 'off';
	const serviceMode = $derived(label(mode, `default (${label(view?.default_mode, '')})`));

	/** Saves the draft; returns the plans it made. Throws the problem when the server refuses it. */
	export async function save(): Promise<Schemas['SyncPlan'][]> {
		const body: Schemas['ServiceWAF'] = {
			mode: (mode || undefined) as Schemas['WAFMode'] | undefined,
			exclusions,
			paths: paths.map((p) => ({
				...p,
				mode: (p.mode || undefined) as Schemas['WAFMode'] | undefined
			}))
		};
		const r = await api.PUT('/api/services/{id}/waf', {
			params: { path: { id: serviceId } },
			body
		});
		if (r.error || !r.data) throw r.error;
		take(r.data.waf);
		return r.data.plans;
	}
</script>

{#if view && !view.editable}
	<Panel title="WAF" description="Default mode, fixed for Hashi’s own services"><span></span></Panel
	>
{:else if view}
	<div class="grid gap-4 lg:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
		<Panel flush title="Mode per path">
			{#snippet help()}<HelpTip topic="WAF modes"
					><p>The most specific path wins.</p>
					<p>
						The WAF checks addresses, headers, cookies and query strings. Request bodies are not
						checked, so uploads, WebDAV and git work everywhere.
					</p></HelpTip
				>{/snippet}
			{#snippet actions()}<Button
					size="sm"
					variant="outline"
					onclick={() =>
						(paths = [...paths, { match: 'prefix', path: '/', mode: '', exclusions: [] }])}
					><Plus />Add path</Button
				>{/snippet}
			<ul class="divide-y text-md">
				<li class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 px-4 py-2.5">
					<span><Mono>/</Mono> <span class="text-muted-foreground">whole service</span></span>
					<Select.Root type="single" bind:value={mode}>
						<Select.Trigger class="w-44" aria-label="WAF mode for the whole service"
							>{serviceMode}</Select.Trigger
						>
						<Select.Content>
							<Select.Item value="">default</Select.Item>
							<Select.Item value="headers_only">headers only</Select.Item>
							<Select.Item value="disabled">off</Select.Item>
						</Select.Content>
					</Select.Root>
				</li>
				{#each paths as p, i (i)}
					<li class="space-y-2 px-4 py-2.5">
						<div class="flex min-w-0 gap-2">
							<Select.Root type="single" bind:value={p.match}>
								<Select.Trigger class="w-24 shrink-0" aria-label="Match">{p.match}</Select.Trigger>
								<Select.Content>
									<Select.Item value="prefix">prefix</Select.Item>
									<Select.Item value="exact">exact</Select.Item>
								</Select.Content>
							</Select.Root>
							<Input bind:value={p.path} class="min-w-0 flex-1 font-mono" aria-label="Path" />
						</div>
						<div class="flex items-center gap-1">
							<Select.Root type="single" bind:value={p.mode}>
								<Select.Trigger
									class="flex-1 sm:w-44 sm:flex-none"
									aria-label="WAF mode for {p.path}">{label(p.mode, 'as service')}</Select.Trigger
								>
								<Select.Content>
									<Select.Item value="">as service</Select.Item>
									<Select.Item value="headers_only">headers only</Select.Item>
									<Select.Item value="disabled">off</Select.Item>
								</Select.Content>
							</Select.Root>
							<Button
								size="icon"
								variant="ghost"
								aria-label="Remove {p.path}"
								onclick={() => (paths = paths.filter((_, j) => j !== i))}><Trash2 /></Button
							>
						</div>
					</li>
				{/each}
			</ul>
		</Panel>
		<Panel flush title="Exclusions">
			{#snippet help()}<HelpTip topic="WAF exclusions"
					><p>A rule switched off here no longer counts against requests to this service.</p>
					<p>
						Use it for a rule that misfires on this app; the other rules keep checking.
					</p></HelpTip
				>{/snippet}
			{#snippet actions()}<WafRulePicker
					{rules}
					chosen={exclusions}
					onpick={(id) => (exclusions = [...exclusions, id].sort((a, b) => a - b))}
				/>{/snippet}
			{#if exclusions.length}
				<ul class="divide-y text-md">
					{#each exclusions as id (id)}
						{@const rule = ruleById.get(id)}
						<li class="flex items-center gap-3 px-4 py-2.5">
							<span class="min-w-0 flex-1">
								<span class="block">{rule?.name ?? `Rule ${id}`}</span>
								<span class="block text-xs text-muted-foreground"
									>{id}{#if rule}{' '}· {rule.group}{:else if rules.length}{' '}· not a rule in this
										WAF version{/if}</span
								>
							</span>
							<Button
								size="icon-sm"
								variant="ghost"
								aria-label="Remove {rule?.name ?? `rule ${id}`}"
								onclick={() => (exclusions = exclusions.filter((x) => x !== id))}><X /></Button
							>
						</li>
					{/each}
				</ul>
			{:else}
				<p data-state class="px-4 py-6 text-md text-muted-foreground">
					Every rule checks this service.
				</p>
			{/if}
		</Panel>
	</div>
{:else if error}
	<p role="alert" class="text-md text-bad-text">{error}</p>
{:else}<Skeleton class="h-40 rounded-2xl" />
{/if}
