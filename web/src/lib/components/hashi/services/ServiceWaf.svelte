<script lang="ts">
	import { onMount } from 'svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Switch } from '$lib/components/ui/switch';
	import * as Select from '$lib/components/ui/select';
	import { Plus, Trash2 } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { pendingPlans } from '$lib/sync-pending.svelte';

	// WAF per path (06 §9): headers only or off. Request bodies are not checked until the plugin's upload bug is fixed,
	// so the mock's "enabled" mode is not offered. Uploads widens what the rules accept (WebDAV, any content type).
	let { serviceId }: { serviceId: string } = $props();

	// A path row as the form edits it: mode '' follows the service.
	type Path = {
		id?: string;
		match: 'prefix' | 'exact';
		path: string;
		mode: string;
		uploads: boolean;
		exclusions: number[];
	};
	let view = $state<Schemas['ServiceWAFView'] | null>(null);
	let mode = $state(''); // '' = the default
	let uploads = $state(false);
	let exclusions = $state('');
	let paths = $state<Path[]>([]);
	let note = $state('');
	let error = $state('');
	let busy = $state(false);

	const ids = (s: string) =>
		s
			.split(/[\s,]+/)
			.filter(Boolean)
			.map(Number);
	function take(v: Schemas['ServiceWAFView']) {
		view = v;
		mode = v.mode ?? '';
		uploads = v.uploads ?? false;
		exclusions = (v.exclusions ?? []).join(', ');
		paths = (v.paths ?? []).map((p) => ({
			id: p.id,
			match: p.match,
			path: p.path,
			mode: p.mode ?? '',
			uploads: p.uploads ?? false,
			exclusions: p.exclusions ?? []
		}));
	}
	onMount(async () => {
		const r = await api.GET('/api/services/{id}/waf', { params: { path: { id: serviceId } } });
		if (r.error || !r.data) error = problemText(r.error);
		else take(r.data);
	});

	const label = (m: string | undefined, inherit: string) =>
		!m ? inherit : m === 'headers_only' ? 'headers only' : 'off';
	const serviceMode = $derived(label(mode, `default (${label(view?.default_mode, '')})`));

	async function save() {
		busy = true;
		error = note = '';
		const body: Schemas['ServiceWAF'] = {
			mode: (mode || undefined) as Schemas['WAFMode'] | undefined,
			uploads,
			exclusions: ids(exclusions),
			paths: paths.map((p) => ({
				...p,
				mode: (p.mode || undefined) as Schemas['WAFMode'] | undefined
			}))
		};
		const r = await api.PUT('/api/services/{id}/waf', {
			params: { path: { id: serviceId } },
			body
		});
		busy = false;
		if (r.error || !r.data) {
			error = problemText(r.error);
			return;
		}
		take(r.data.waf);
		const waiting = r.data.plans.filter((p) => p.status === 'pending').length;
		pendingPlans.count += waiting;
		note = waiting
			? `Saved. ${waiting} change${waiting === 1 ? '' : 's'} wait under Pending changes.`
			: 'Saved and applied.';
	}
</script>

{#if view && !view.editable}
	<Panel title="WAF">
		<p class="text-[13px] text-muted-foreground">
			{view.uploads
				? 'Hashi’s own services use the default mode with uploads allowed; they change only through their settings.'
				: 'TCP and UDP services are protected at the firewall.'}
		</p>
	</Panel>
{:else if view}
	<div class="grid gap-4 lg:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
		<Panel
			flush
			title="Mode per path"
			description="The most specific path wins. Saving plans every Traefik host."
		>
			{#snippet actions()}<Button
					size="sm"
					variant="outline"
					onclick={() =>
						(paths = [
							...paths,
							{ match: 'prefix', path: '/', mode: '', uploads: false, exclusions: [] }
						])}><Plus />Add path</Button
				>{/snippet}
			<ul class="divide-y text-[13px]">
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
						<label class="flex items-center gap-2 text-muted-foreground"
							><Switch bind:checked={p.uploads} />Uploads and WebDAV on this path</label
						>
					</li>
				{/each}
			</ul>
			<label class="flex items-center gap-2 border-t px-4 py-3 text-[13px]"
				><Switch bind:checked={uploads} />Uploads and WebDAV on the whole service</label
			>
			<p class="border-t px-4 py-3 text-[12.5px] text-muted-foreground">
				The WAF checks addresses, headers, cookies and query strings. Request bodies are not checked
				yet. Turn on uploads where files are sent (file sharing, git pushes, WebDAV).
			</p>
		</Panel>
		<Panel title="Exclusions" description="Rules switched off where they misfire">
			<div class="space-y-1.5">
				<Input
					bind:value={exclusions}
					placeholder="e.g. 942100, 920350"
					class="font-mono"
					aria-label="Rule IDs"
				/>
				<p class="text-[12.5px] text-muted-foreground">
					Rule IDs for the whole service. Matches count towards a challenge.
				</p>
			</div>
			{#if error}<p role="alert" class="mt-3 text-[13px] text-bad-text">{error}</p>{/if}
			{#if note}<p role="status" class="mt-3 text-[13px]">{note}</p>{/if}
			<Button class="mt-4" size="sm" disabled={busy} onclick={save}>Save WAF</Button>
		</Panel>
	</div>
{:else if error}
	<p role="alert" class="text-[13px] text-bad-text">{error}</p>
{/if}
