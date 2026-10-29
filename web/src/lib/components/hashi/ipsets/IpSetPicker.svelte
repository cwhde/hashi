<script lang="ts">
	import Tag from '../Tag.svelte';
	import * as Popover from '$lib/components/ui/popover';
	import * as Command from '$lib/components/ui/command';
	import { Button } from '$lib/components/ui/button';
	import { ChevronsUpDown, Check, Lock } from '@lucide/svelte';
	import { kindLabel, type IPSet } from '$lib/ipsets';

	// Picks one IP set by id. In target positions (DNS values, upstreams, connections) only address sets can be
	// chosen; the others stay visible with the reason (05a §5.2). `exclude` hides sets that cannot be picked here.
	let {
		value = $bindable(''),
		sets,
		addressOnly = false,
		exclude = [],
		id
	}: {
		value?: string;
		sets: IPSet[];
		addressOnly?: boolean;
		exclude?: string[];
		id?: string;
	} = $props();
	let open = $state(false);
	const shown = $derived(sets.filter((s) => !exclude.includes(s.id)));
	const current = $derived(sets.find((s) => s.id === value));
	const size = (s: IPSet) =>
		`${s.v4_count.toLocaleString()} v4 · ${s.v6_count.toLocaleString()} v6`;
</script>

<Popover.Root bind:open>
	<Popover.Trigger>
		{#snippet child({ props })}
			<Button {...props} {id} variant="outline" class="w-full justify-between font-normal"
				>{current?.slug ?? 'Choose a set'}<ChevronsUpDown class="text-muted-foreground" /></Button
			>
		{/snippet}
	</Popover.Trigger>
	<Popover.Content class="w-[min(26rem,calc(100vw-2rem))] p-0" align="start">
		<Command.Root>
			<Command.Input placeholder="Find a set" />
			<Command.List>
				<Command.Empty>No set matches.</Command.Empty>
				{#each ['user', 'system', 'feed', 'dynamic'] as const as kind (kind)}
					{@const group = shown.filter((s) => s.kind === kind)}
					{#if group.length}
						<Command.Group heading={kind === 'user' ? 'Your sets' : kindLabel[kind]}>
							{#each group as s (s.id)}
								{@const blocked = addressOnly && !s.address_set}
								<Command.Item
									value={s.slug + ' ' + s.name}
									disabled={blocked}
									onSelect={() => {
										value = s.id;
										open = false;
									}}
								>
									<Check class="size-4 {value === s.id ? '' : 'opacity-0'}" />
									<span class="min-w-0 flex-1">
										<span class="flex items-center gap-1.5"
											>{s.slug}{#if !s.editable}<Lock
													class="size-3 text-muted-foreground"
												/>{/if}</span
										>
										<span class="block text-[12px] text-muted-foreground"
											>{blocked ? 'Not an address set: has ranges or countries' : size(s)}</span
										>
									</span>
									{#if s.resolution_status === 'partial'}<Tag tone="warn">partial</Tag>{/if}
								</Command.Item>
							{/each}
						</Command.Group>
					{/if}
				{/each}
			</Command.List>
		</Command.Root>
	</Popover.Content>
</Popover.Root>
