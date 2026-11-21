<script lang="ts">
	import CardColumns from '#lib/components/hashi/CardColumns.svelte';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import Mono from '#lib/components/hashi/Mono.svelte';
	import MiddlewarePicker from './MiddlewarePicker.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import { Switch } from '#lib/components/ui/switch/index.ts';
	import * as RadioGroup from '#lib/components/ui/radio-group/index.ts';
	import { Plus, Trash2 } from '@lucide/svelte';
	import type { Schemas } from '#lib/api/client.ts';

	// What the app behind a service receives (04 §1.1a): Host header, visitor address headers, extra headers, timeouts,
	// extra middlewares. Edited here, saved with "Save and publish" like the rest of the service.
	type Traffic = Schemas['ServiceTraffic'];
	type Header = Schemas['ServiceHeader'];
	let {
		traffic = $bindable(),
		middlewares = $bindable([]),
		names,
		every,
		publicName,
		upstream,
		disabled = false
	}: {
		traffic: Traffic;
		middlewares?: string[];
		names: string[];
		every: string[];
		publicName: string;
		upstream: string;
		disabled?: boolean;
	} = $props();

	const hostModes = [
		{ v: '', label: 'The public name', help: () => publicName || 'the service domain' },
		{
			v: 'upstream',
			label: 'The upstream address',
			help: () => upstream + ', as written in the target'
		},
		{ v: 'custom', label: 'A name you choose', help: () => '' }
	] as const;
	const hostMode = $derived(traffic.host ?? '');

	function setHost(v: string) {
		traffic.host = v as Traffic['host'];
		if (v !== 'custom') traffic.host_value = undefined;
	}
	function addHeader(which: 'request_headers' | 'response_headers') {
		traffic[which] = [...(traffic[which] ?? []), { name: '', value: '' }];
	}
	function dropHeader(which: 'request_headers' | 'response_headers', i: number) {
		traffic[which] = (traffic[which] ?? []).filter((_, j) => j !== i);
	}
	const seconds = (v: number | undefined) => (v ? String(v) : '');
	function setSeconds(k: 'dial_timeout' | 'response_header_timeout' | 'idle_timeout', v: string) {
		const n = Number(v.trim());
		traffic[k] = v.trim() && Number.isFinite(n) ? n : undefined;
	}
	const timeouts = [
		['dial_timeout', 'Connect within (seconds)'],
		['response_header_timeout', 'Wait for an answer (seconds)'],
		['idle_timeout', 'Keep idle connections (seconds)']
	] as const;
</script>

{#snippet headerList(which: 'request_headers' | 'response_headers', rows: Header[])}
	{#if rows.length}
		<ul
			class="space-y-2"
			aria-label={which === 'request_headers' ? 'Headers to the app' : 'Headers to the visitor'}
		>
			{#each rows as h, i (i)}
				<li class="grid grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)_auto] gap-2">
					<Input
						bind:value={h.name}
						class="font-mono"
						aria-label="Header name"
						placeholder="X-Example"
					/>
					<Input
						bind:value={h.value}
						class="font-mono"
						aria-label="Header value"
						placeholder="value"
					/>
					<Button
						variant="ghost"
						size="icon-sm"
						class="self-center"
						aria-label="Remove {h.name || 'header'}"
						onclick={() => dropHeader(which, i)}><Trash2 /></Button
					>
				</li>
			{/each}
		</ul>
	{/if}
	<Button variant="outline" size="sm" onclick={() => addHeader(which)}><Plus />Add header</Button>
{/snippet}

<fieldset {disabled}>
	<CardColumns>
		{#snippet left()}
			<Panel title="Host header">
				<RadioGroup.Root
					value={hostMode}
					onValueChange={setHost}
					class="gap-2"
					aria-label="Host header"
				>
					{#each hostModes as m (m.v)}
						<label
							class="flex cursor-pointer gap-3 rounded-xl border p-3 hover:bg-surface-2/50 {hostMode ===
							m.v
								? 'border-primary/60'
								: ''}"
						>
							<RadioGroup.Item value={m.v} class="mt-0.5" />
							<span class="min-w-0"
								><span class="block text-md font-medium">{m.label}</span>{#if m.help()}<span
										class="block text-sm break-all text-muted-foreground">{m.help()}</span
									>{/if}</span
							>
						</label>
					{/each}
				</RadioGroup.Root>
				{#if hostMode === 'custom'}
					<div class="mt-3 space-y-1.5">
						<Label for="hostv">Name sent to the app</Label>
						<Input
							id="hostv"
							bind:value={traffic.host_value}
							class="font-mono"
							placeholder="app.internal.example"
						/>
					</div>
				{/if}
			</Panel>
			<Panel title="Extra headers to the app">
				<div class="space-y-3">
					{@render headerList('request_headers', traffic.request_headers ?? [])}
				</div>
			</Panel>
			<Panel title="Timeouts">
				<div class="grid gap-3 sm:grid-cols-3">
					{#each timeouts as [k, label] (k)}
						<div class="space-y-1.5">
							<Label for={k}>{label}</Label>
							<Input
								id={k}
								inputmode="numeric"
								value={seconds(traffic[k])}
								oninput={(e) => setSeconds(k, e.currentTarget.value)}
								placeholder="default"
							/>
						</div>
					{/each}
				</div>
			</Panel>
		{/snippet}
		{#snippet right()}
			<Panel title="Visitor address">
				<div class="space-y-3">
					<div class="flex items-center justify-between gap-3">
						<Label for="xff">Send <Mono>X-Forwarded-For</Mono></Label>
						<Switch
							id="xff"
							checked={!traffic.drop_forwarded_for}
							onCheckedChange={(on) => (traffic.drop_forwarded_for = !on || undefined)}
						/>
					</div>
					<div class="flex items-center justify-between gap-3">
						<Label for="xri">Send <Mono>X-Real-IP</Mono></Label>
						<Switch
							id="xri"
							checked={!traffic.drop_real_ip}
							onCheckedChange={(on) => (traffic.drop_real_ip = !on || undefined)}
						/>
					</div>
				</div>
			</Panel>
			<Panel title="Extra headers to the visitor">
				<div class="space-y-3">
					{@render headerList('response_headers', traffic.response_headers ?? [])}
				</div>
			</Panel>
			<Panel title="Extra middlewares">
				<div class="space-y-3">
					{#if names.length || middlewares.length}
						<MiddlewarePicker
							{names}
							bind:selected={middlewares}
							label="Extra middlewares"
							{disabled}
						/>
					{/if}
					{#if every.length}
						<p class="text-sm text-muted-foreground">
							Already on every service: {#each every as n, i (n)}{i ? ', ' : ''}<Mono>{n}</Mono
								>{/each}
						</p>
					{/if}
					<Button size="sm" variant="outline" href="/traefik?tab=middlewares"
						><Plus />Add middleware</Button
					>
				</div>
			</Panel>
		{/snippet}
	</CardColumns>
</fieldset>
