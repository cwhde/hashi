<script lang="ts">
	import Tag from '../Tag.svelte';
	import * as Popover from '$lib/components/ui/popover';
	import * as Command from '$lib/components/ui/command';
	import { Button } from '$lib/components/ui/button';
	import { ipSets } from '$lib/design/security';
	import { ChevronsUpDown, Check, Lock } from '@lucide/svelte';

	// Picks one IP set. In target positions (DNS values, upstreams, connections) only address sets can be chosen;
	// the others stay visible with the reason (05a §5.2).
	let {
		value = $bindable('all'),
		addressOnly = false,
		id
	}: { value?: string; addressOnly?: boolean; id?: string } = $props();
	let open = $state(false);
	const kindLabel = { user: 'yours', system: 'system', feed: 'feed', dynamic: 'dynamic' };
	const size = (v4: number, v6: number) => `${v4.toLocaleString()} v4 · ${v6.toLocaleString()} v6`;
</script>

<Popover.Root bind:open>
	<Popover.Trigger>
		{#snippet child({ props })}
			<Button {...props} {id} variant="outline" class="w-full justify-between font-normal"
				>{value}<ChevronsUpDown class="text-muted-foreground" /></Button
			>
		{/snippet}
	</Popover.Trigger>
	<Popover.Content class="w-[min(26rem,calc(100vw-2rem))] p-0" align="start">
		<Command.Root>
			<Command.Input placeholder="Find a set or an address" />
			<Command.List>
				<Command.Empty>No set matches.</Command.Empty>
				{#each ['user', 'system', 'feed', 'dynamic'] as const as kind (kind)}
					<Command.Group heading={kind === 'user' ? 'Your sets' : kindLabel[kind]}>
						{#each ipSets.filter((s) => s.kind === kind) as s (s.id)}
							{@const blocked = addressOnly && !s.addressSet}
							<Command.Item
								value={s.slug}
								disabled={blocked}
								onSelect={() => {
									value = s.slug;
									open = false;
								}}
							>
								<Check class="size-4 {value === s.slug ? '' : 'opacity-0'}" />
								<span class="min-w-0 flex-1">
									<span class="flex items-center gap-1.5"
										>{s.slug}{#if s.locked}<Lock class="size-3 text-muted-foreground" />{/if}</span
									>
									<span class="block text-[12px] text-muted-foreground"
										>{blocked
											? 'Not an address set: has ranges or countries'
											: size(s.v4, s.v6)}</span
									>
								</span>
								{#if s.status === 'partial'}<Tag tone="warn">partial</Tag>{/if}
							</Command.Item>
						{/each}
					</Command.Group>
				{/each}
			</Command.List>
		</Command.Root>
	</Popover.Content>
</Popover.Root>
