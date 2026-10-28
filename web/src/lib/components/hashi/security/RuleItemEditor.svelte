<script lang="ts">
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import * as Select from '$lib/components/ui/select';
	import * as RadioGroup from '$lib/components/ui/radio-group';
	import IpSetPicker from './IpSetPicker.svelte';
	import { actionMeta, type Action } from '$lib/design/security';

	// One rule item: who (an IP set), which paths, what happens (06 §4.1).
	let {
		appliesTo = $bindable('all'),
		action = $bindable<Action>('pass_captcha')
	}: { appliesTo?: string; action?: Action } = $props();
	let condType = $state('prefix');
	const actions = Object.keys(actionMeta) as Action[];
</script>

<div class="space-y-4">
	<div class="space-y-1.5">
		<Label for="at">Applies to</Label>
		<IpSetPicker id="at" bind:value={appliesTo} />
	</div>
	<div class="space-y-1.5">
		<Label for="pp">Path</Label>
		<div class="grid grid-cols-[8rem_1fr] gap-2">
			<Select.Root type="single" bind:value={condType}>
				<Select.Trigger class="w-full" aria-label="Path match">{condType}</Select.Trigger>
				<Select.Content>
					<Select.Item value="prefix">prefix</Select.Item>
					<Select.Item value="exact">exact</Select.Item>
					<Select.Item value="regex">regex</Select.Item>
				</Select.Content>
			</Select.Root>
			<Input id="pp" class="font-mono" placeholder="/ (every path)" />
		</div>
	</div>
	<fieldset class="space-y-2">
		<legend class="mb-1.5 text-[13px] font-medium">Action</legend>
		<RadioGroup.Root bind:value={action} class="gap-2">
			{#each actions as a (a)}
				<label
					class="flex cursor-pointer gap-3 rounded-xl border p-3 hover:bg-surface-2/50 {action === a
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
	{#if action === 'pass_authentication'}
		<div class="grid gap-3 rounded-xl border p-3 sm:grid-cols-2">
			<div class="space-y-1.5">
				<Label>Provider</Label>
				<Select.Root type="single" value="default">
					<Select.Trigger class="w-full">Pocket ID (default)</Select.Trigger>
					<Select.Content
						><Select.Item value="default">Pocket ID (default)</Select.Item></Select.Content
					>
				</Select.Root>
			</div>
			<div class="space-y-1.5">
				<Label for="gr">Allowed groups</Label><Input id="gr" placeholder="Anyone who signs in" />
			</div>
		</div>
	{/if}
	<div class="grid gap-3 sm:grid-cols-[1fr_auto] sm:items-end">
		<div class="space-y-1.5">
			<Label for="rn">Note</Label><Input id="rn" placeholder="Optional" />
		</div>
		<label class="flex h-8 items-center gap-2 text-[13px]"><Switch checked />Enabled</label>
	</div>
</div>
