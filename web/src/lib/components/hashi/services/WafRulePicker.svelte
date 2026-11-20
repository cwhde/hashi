<script lang="ts">
	import * as Popover from '#lib/components/ui/popover/index.ts';
	import * as Command from '#lib/components/ui/command/index.ts';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Plus } from '@lucide/svelte';
	import type { Schemas } from '#lib/api/client.ts';

	// Adds a CRS rule to the exclusions by its name or number (bug 18). The names are the rules' own messages, read by
	// the server from the rule files the WAF runs.
	let {
		rules,
		chosen,
		onpick
	}: { rules: Schemas['WAFRule'][]; chosen: number[]; onpick: (id: number) => void } = $props();
	let open = $state(false);
	const groups = $derived(
		Object.entries(
			Object.groupBy(
				rules.filter((r) => !chosen.includes(r.id)),
				(r) => r.group
			)
		)
	);
</script>

<Popover.Root bind:open>
	<Popover.Trigger>
		{#snippet child({ props })}
			<Button {...props} size="sm" variant="outline"><Plus />Add rule</Button>
		{/snippet}
	</Popover.Trigger>
	<Popover.Content class="w-[min(30rem,calc(100vw-2rem))] p-0" align="end">
		<Command.Root label="Search rules">
			<Command.Input placeholder="Search by name or number" />
			<Command.List class="max-h-80">
				<Command.Empty>No rule matches.</Command.Empty>
				{#each groups as [group, list] (group)}
					<Command.Group heading={group}>
						{#each list ?? [] as r (r.id)}
							<Command.Item
								value={`${r.id} ${r.name} ${r.group}`}
								onSelect={() => {
									onpick(r.id);
									open = false;
								}}
							>
								<span class="min-w-0 flex-1">
									<span class="block">{r.name}</span>
									<span class="block text-xs text-muted-foreground"
										>{r.id}{#if r.paranoia > 1}{' '}· paranoia level {r.paranoia}{/if}</span
									>
								</span>
							</Command.Item>
						{/each}
					</Command.Group>
				{/each}
			</Command.List>
		</Command.Root>
	</Popover.Content>
</Popover.Root>
