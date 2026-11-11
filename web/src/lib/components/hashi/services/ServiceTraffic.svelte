<script lang="ts">
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import MiddlewarePicker from './MiddlewarePicker.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import * as RadioGroup from '$lib/components/ui/radio-group';
	import { Plus, Trash2 } from '@lucide/svelte';
	import type { Schemas } from '$lib/api/client';

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
		{
			v: 'custom',
			label: 'A name you choose',
			help: () => 'for apps that only answer to their own name'
		}
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
		['dial_timeout', 'Connect', 'How long to wait for the app to accept a connection.'],
		[
			'response_header_timeout',
			'Wait for an answer',
			'Slower answers end with 504. Leave empty for no limit.'
		],
		[
			'idle_timeout',
			'Keep idle connections',
			'How long an unused connection to the app stays open.'
		]
	] as const;
</script>

{#snippet headerList(which: 'request_headers' | 'response_headers', rows: Header[], hint: string)}
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
						placeholder="empty removes it"
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
	<div class="flex flex-wrap items-center justify-between gap-2">
		<p class="text-[12.5px] text-muted-foreground">{hint}</p>
		<Button variant="ghost" size="sm" onclick={() => addHeader(which)}><Plus />Add header</Button>
	</div>
{/snippet}

<fieldset class="grid gap-4 lg:grid-cols-2" {disabled}>
	<Panel title="Host header" description="The name the app sees in each request">
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
						><span class="block text-[13.5px] font-medium">{m.label}</span><span
							class="block text-[12.5px] break-all text-muted-foreground">{m.help()}</span
						></span
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
		<p class="mt-3 text-[12.5px] text-muted-foreground">
			<Mono>X-Forwarded-Host</Mono> always carries the public name.
		</p>
	</Panel>

	<Panel title="Visitor address" description="Headers that tell the app who is asking">
		<div class="space-y-3">
			<div class="flex items-center justify-between gap-3">
				<Label for="xff" class="block"
					><span class="block">Send <Mono>X-Forwarded-For</Mono></span><span
						class="block text-[12.5px] font-normal text-muted-foreground"
						>The visitor's address, plus every proxy on the way</span
					></Label
				>
				<Switch
					id="xff"
					checked={!traffic.drop_forwarded_for}
					onCheckedChange={(on) => (traffic.drop_forwarded_for = !on || undefined)}
				/>
			</div>
			<div class="flex items-center justify-between gap-3">
				<Label for="xri" class="block"
					><span class="block">Send <Mono>X-Real-IP</Mono></span><span
						class="block text-[12.5px] font-normal text-muted-foreground"
						>The visitor's address alone</span
					></Label
				>
				<Switch
					id="xri"
					checked={!traffic.drop_real_ip}
					onCheckedChange={(on) => (traffic.drop_real_ip = !on || undefined)}
				/>
			</div>
			<p class="text-[12.5px] text-muted-foreground">
				Hashi's own checks always see the visitor's address; this only changes what the app gets.
			</p>
		</div>
	</Panel>

	<Panel title="Extra headers to the app">
		<div class="space-y-3">
			{@render headerList(
				'request_headers',
				traffic.request_headers ?? [],
				'Set a header on every request. An empty value removes it.'
			)}
		</div>
	</Panel>

	<Panel title="Extra headers to the visitor">
		<div class="space-y-3">
			{@render headerList(
				'response_headers',
				traffic.response_headers ?? [],
				"Set a header on every answer. An empty value removes the app's own."
			)}
		</div>
	</Panel>

	<Panel title="Timeouts" description="In seconds; empty keeps Traefik's default">
		<div class="grid gap-3 sm:grid-cols-3">
			{#each timeouts as [k, label, help] (k)}
				<div class="space-y-1.5">
					<Label for={k}>{label}</Label>
					<Input
						id={k}
						inputmode="numeric"
						value={seconds(traffic[k])}
						oninput={(e) => setSeconds(k, e.currentTarget.value)}
						placeholder="default"
					/>
					<p class="text-[12px] text-muted-foreground">{help}</p>
				</div>
			{/each}
		</div>
	</Panel>

	<Panel title="Extra middlewares" description="They run in the order you pick them">
		{#if names.length || middlewares.length}
			<MiddlewarePicker {names} bind:selected={middlewares} label="Extra middlewares" {disabled} />
		{:else}
			<p class="text-[13px] text-muted-foreground">
				None written yet. Write them on the <a
					class="underline underline-offset-2"
					href="/traefik?tab=middlewares">Traefik page</a
				>.
			</p>
		{/if}
		{#if every.length}
			<p class="mt-3 text-[12.5px] text-muted-foreground">
				On every service already: {#each every as n, i (n)}{i ? ', ' : ''}<Mono>{n}</Mono>{/each}.
			</p>
		{/if}
	</Panel>
</fieldset>
