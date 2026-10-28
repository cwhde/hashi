<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { Textarea } from '$lib/components/ui/textarea';
	import * as Select from '$lib/components/ui/select';
	import * as Collapsible from '$lib/components/ui/collapsible';
	import { ChevronDown } from '@lucide/svelte';
	import type { Schemas } from '$lib/api/client';

	// Inline editor for a manual or imported entry (03 §7, mock: /design/dns). New entries pick name and type; existing
	// ones keep both (the RRSet's identity) and change TTL, values and display fields.
	type Entry = Schemas['DNSRecord'];
	export type EntryForm = {
		name: string;
		type: Schemas['DNSRecordInput']['type'];
		ttl?: number;
		values: string[];
		display_name: string;
		dashboard: boolean;
		monitoring: boolean;
	};

	let {
		entry,
		zone,
		busy = false,
		onSave,
		onDelete,
		onCancel
	}: {
		entry?: Entry;
		zone: string;
		busy?: boolean;
		onSave: (f: EntryForm) => void;
		onDelete?: () => void;
		onCancel: () => void;
	} = $props();

	const types = ['A', 'AAAA', 'CNAME', 'MX', 'TXT', 'SRV', 'CAA'] as const;
	const hints: Record<EntryForm['type'], string> = {
		A: 'One IPv4 address per line',
		AAAA: 'One IPv6 address per line',
		CNAME: 'One target host, e.g. app.example.net',
		MX: 'priority target, e.g. 10 mail.example.net',
		TXT: 'Plain text, one value per line; Hashi adds the quotes',
		SRV: 'priority weight port target, e.g. 10 5 443 host.example.net (name starts with _service._tcp)',
		CAA: 'flags tag "value", e.g. 0 issue "letsencrypt.org"'
	};

	// The form starts from the entry once; later changes to the prop do not reset what the admin typed.
	const initial = () => ({
		name: entry?.name ?? '',
		type: (entry?.type ?? 'A') as EntryForm['type'],
		ttl: entry?.ttl != null ? String(entry.ttl) : '',
		values: entry?.values.join('\n') ?? '',
		display: entry?.display_name ?? '',
		dashboard: entry?.dashboard ?? false,
		monitoring: entry?.monitoring ?? false
	});
	let f = $state(initial());

	function submit(e: SubmitEvent) {
		e.preventDefault();
		onSave({
			name: f.name.trim() || '@',
			type: f.type,
			ttl: f.ttl.trim() ? Number(f.ttl) : undefined,
			values: f.values
				.split('\n')
				.map((v) => v.trim())
				.filter(Boolean),
			display_name: f.display.trim(),
			dashboard: f.dashboard,
			monitoring: f.monitoring
		});
	}
</script>

<form class="border-b bg-surface-2/40 px-4 pt-3 pb-4" onsubmit={submit}>
	<div class="grid gap-3 sm:grid-cols-[minmax(8rem,1fr)_7rem_7rem]">
		<div class="space-y-1">
			<Label for="rec-name" class="text-[12px]">Name</Label>
			<div class="flex items-center gap-1.5">
				<Input
					id="rec-name"
					bind:value={f.name}
					disabled={!!entry}
					placeholder="@ for {zone}"
					class="font-mono"
				/>
				<span class="hidden text-[12.5px] text-muted-foreground sm:inline">.{zone}</span>
			</div>
		</div>
		<div class="space-y-1">
			<Label class="text-[12px]">Type</Label>
			<Select.Root type="single" bind:value={f.type} disabled={!!entry}>
				<Select.Trigger class="w-full" aria-label="Type">{f.type}</Select.Trigger>
				<Select.Content>
					{#each types as t (t)}<Select.Item value={t}>{t}</Select.Item>{/each}
				</Select.Content>
			</Select.Root>
		</div>
		<div class="space-y-1">
			<Label for="rec-ttl" class="text-[12px]">TTL (seconds)</Label>
			<Input
				id="rec-ttl"
				bind:value={f.ttl}
				inputmode="numeric"
				placeholder="zone default"
				class="tnum"
			/>
		</div>
	</div>
	<div class="mt-3 space-y-1">
		<Label for="rec-values" class="text-[12px]">Values</Label>
		<Textarea
			id="rec-values"
			bind:value={f.values}
			rows={Math.max(2, f.values.split('\n').length)}
			class="font-mono"
			required
		/>
		<p class="text-[12px] text-muted-foreground">{hints[f.type]}</p>
	</div>
	<Collapsible.Root class="mt-3">
		<Collapsible.Trigger
			class="flex items-center gap-1 text-[12.5px] text-muted-foreground hover:text-foreground"
			><ChevronDown class="size-3.5" />Display, monitoring and dashboard</Collapsible.Trigger
		>
		<Collapsible.Content class="mt-2 grid gap-3 sm:grid-cols-3">
			<div class="space-y-1">
				<Label for="rec-display" class="text-[12px]">Display name</Label>
				<Input id="rec-display" bind:value={f.display} />
			</div>
			<label class="flex items-center gap-2 pt-5 text-[13px]"
				><Switch bind:checked={f.monitoring} />Monitor</label
			>
			<label class="flex items-center gap-2 pt-5 text-[13px]"
				><Switch bind:checked={f.dashboard} />Show on app dashboard</label
			>
		</Collapsible.Content>
	</Collapsible.Root>
	<div class="mt-3 flex flex-wrap items-center gap-2">
		{#if onDelete}
			<Button
				type="button"
				variant="ghost"
				size="sm"
				class="text-bad-text"
				disabled={busy}
				onclick={onDelete}>Delete record</Button
			>
		{/if}
		<span class="flex-1"></span>
		<Button type="button" variant="ghost" size="sm" onclick={onCancel}>Cancel</Button>
		<Button type="submit" size="sm" disabled={busy}
			>{busy ? 'Saving…' : 'Save · creates a plan'}</Button
		>
	</div>
</form>
