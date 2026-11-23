<script lang="ts">
	import Tag from '../Tag.svelte';
	import * as Popover from '#lib/components/ui/popover/index.ts';
	import * as Command from '#lib/components/ui/command/index.ts';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { ChevronsUpDown, Check, Lock, Plus } from '@lucide/svelte';
	import { kindLabel, type IPSet } from '#lib/ipsets.ts';

	// Picks one IP set by id. In target positions (DNS values, upstreams, connections) only address sets can be
	// chosen; the others stay visible with the reason (05a §5.2). `exclude` hides sets that cannot be picked here.
	let {
		value = $bindable(''),
		sets,
		addressOnly = false,
		exclude = [],
		id,
		oncreate
	}: {
		value?: string;
		sets: IPSet[];
		addressOnly?: boolean;
		exclude?: string[];
		id?: string;
		/** When given, the list ends with "New set": it gets the search text as a set name. */
		oncreate?: (typed: string) => void;
	} = $props();
	let open = $state(false);
	let query = $state('');
	/** What was typed, as a set name: lowercase letters, digits and dashes. */
	const typedSlug = $derived(
		query
			.trim()
			.toLowerCase()
			.replace(/[^a-z0-9-]+/g, '-')
			.replace(/^-+|-+$/g, '')
	);
	const shown = $derived(sets.filter((s) => !exclude.includes(s.id)));
	const current = $derived(sets.find((s) => s.id === value));
	const size = (s: IPSet) =>
		`${s.v4_count.toLocaleString()} v4 · ${s.v6_count.toLocaleString()} v6`;
</script>

<!-- Each opening starts a new search, as before the search text was bound for "New set" (R4-003). -->
<Popover.Root bind:open onOpenChange={(o) => o && (query = '')}>
	<Popover.Trigger>
		{#snippet child({ props })}
			<Button {...props} {id} variant="outline" class="w-full justify-between font-normal"
				>{current?.slug ?? 'Choose a set'}<ChevronsUpDown class="text-muted-foreground" /></Button
			>
		{/snippet}
	</Popover.Trigger>
	<Popover.Content class="w-[min(26rem,calc(100vw-2rem))] p-0" align="start">
		<Command.Root label="Find a set">
			<Command.Input placeholder="Find a set" bind:value={query} />
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
										<span class="block text-xs text-muted-foreground"
											>{blocked ? 'Not an address set: has ranges or countries' : size(s)}</span
										>
									</span>
									{#if s.resolution_status === 'partial'}<Tag tone="warn">partial</Tag>{/if}
								</Command.Item>
							{/each}
						</Command.Group>
					{/if}
				{/each}
				{#if oncreate}
					<Command.Group forceMount>
						<Command.Item
							forceMount
							value="new set"
							onSelect={() => {
								open = false;
								oncreate(typedSlug);
							}}
						>
							<Plus class="size-4" />New set{#if typedSlug}{' '}<span class="font-mono"
									>{typedSlug}</span
								>{/if}
						</Command.Item>
					</Command.Group>
				{/if}
			</Command.List>
		</Command.Root>
	</Popover.Content>
</Popover.Root>
