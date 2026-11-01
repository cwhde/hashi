<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import * as Select from '$lib/components/ui/select';
	import * as RadioGroup from '$lib/components/ui/radio-group';
	import IpSetPicker from '../ipsets/IpSetPicker.svelte';
	import { actionMeta, actions, type Provider, type RuleEntry } from '$lib/rules';
	import type { IPSet } from '$lib/ipsets';

	// One rule item: who (an IP set), which paths, what happens (06 §4.1).
	let {
		rule = $bindable(),
		sets,
		providers
	}: { rule: RuleEntry; sets: IPSet[]; providers: Provider[] } = $props();
	const match = $derived(rule.match_type || 'any');
	const provider = $derived(providers.find((p) => p.id === rule.provider_id));
	const fallbackName = $derived(providers.find((p) => p.is_default)?.name ?? 'No default provider');
</script>

<div class="space-y-4">
	<div class="space-y-1.5">
		<Label for="at">Applies to</Label>
		<IpSetPicker id="at" bind:value={rule.applies_to} {sets} />
	</div>
	<div class="space-y-1.5">
		<Label for="pp">Path</Label>
		<div class="grid grid-cols-[8rem_1fr] gap-2">
			<Select.Root
				type="single"
				value={match}
				onValueChange={(v) => (rule.match_type = v === 'any' ? '' : (v as RuleEntry['match_type']))}
			>
				<Select.Trigger class="w-full" aria-label="Path match">{match}</Select.Trigger>
				<Select.Content>
					{#each ['any', 'prefix', 'exact', 'regex'] as m (m)}<Select.Item value={m}
							>{m}</Select.Item
						>{/each}
				</Select.Content>
			</Select.Root>
			<Input
				id="pp"
				class="font-mono"
				disabled={match === 'any'}
				placeholder={match === 'any' ? 'every path' : '/admin'}
				bind:value={rule.pattern}
			/>
		</div>
	</div>
	<fieldset class="space-y-2">
		<legend class="mb-1.5 text-[13px] font-medium">Action</legend>
		<RadioGroup.Root bind:value={rule.action} class="gap-2">
			{#each actions as a (a)}
				<label
					class="flex cursor-pointer gap-3 rounded-xl border p-3 hover:bg-surface-2/50 {rule.action ===
					a
						? 'border-primary/60'
						: ''}"
				>
					<RadioGroup.Item value={a} class="mt-0.5" />
					<span
						><span class="block text-[13.5px] font-medium">{actionMeta[a].label}</span><span
							class="block text-[12.5px] text-muted-foreground">{actionMeta[a].help}</span
						></span
					>
				</label>
			{/each}
		</RadioGroup.Root>
	</fieldset>
	{#if rule.action === 'pass_authentication'}
		<div class="grid gap-3 rounded-xl border p-3 sm:grid-cols-2">
			<div class="space-y-1.5">
				<Label>Provider</Label>
				<Select.Root
					type="single"
					value={rule.provider_id || 'default'}
					onValueChange={(v) => (rule.provider_id = v === 'default' ? '' : v)}
				>
					<Select.Trigger class="w-full"
						>{provider?.name ?? `Default: ${fallbackName}`}</Select.Trigger
					>
					<Select.Content>
						<Select.Item value="default">Default: {fallbackName}</Select.Item>
						{#each providers as p (p.id)}<Select.Item value={p.id}>{p.name}</Select.Item>{/each}
					</Select.Content>
				</Select.Root>
			</div>
			<div class="space-y-1.5">
				<Label for="gr">Allowed groups</Label>
				<Input
					id="gr"
					placeholder="Anyone who signs in"
					value={rule.groups.join(', ')}
					oninput={(e) =>
						(rule.groups = e.currentTarget.value
							.split(',')
							.map((g) => g.trim())
							.filter(Boolean))}
				/>
			</div>
		</div>
	{/if}
	<div class="grid gap-3 sm:grid-cols-[1fr_auto] sm:items-end">
		<div class="space-y-1.5">
			<Label for="rn">Note</Label><Input id="rn" placeholder="Optional" bind:value={rule.note} />
		</div>
		<label class="flex h-8 items-center gap-2 text-[13px]"
			><Switch bind:checked={rule.enabled} />Enabled</label
		>
	</div>
</div>
