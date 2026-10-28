<script lang="ts">
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import { Button } from '$lib/components/ui/button';
	import * as Table from '$lib/components/ui/table';
	import { ruleSets } from '$lib/design/security';
	import { Plus, Lock, TriangleAlert } from '@lucide/svelte';
</script>

<PageHeader
	title="Rule sets"
	description="Reusable lists of rules. A service runs its own rules first, then the global set, then its fallback. The first rule that matches decides."
>
	{#snippet actions()}<Button><Plus />Add rule set</Button>{/snippet}
</PageHeader>

<div class="rounded-2xl border bg-card">
	<Table.Root>
		<Table.Header>
			<Table.Row>
				<Table.Head class="pl-4">Rule set</Table.Head>
				<Table.Head class="hidden text-right sm:table-cell">Entries</Table.Head>
				<Table.Head class="hidden md:table-cell">Used by</Table.Head>
				<Table.Head>Warnings</Table.Head>
			</Table.Row>
		</Table.Header>
		<Table.Body>
			{#each ruleSets as r (r.id)}
				<Table.Row>
					<Table.Cell class="max-w-[30rem] pl-4">
						<a href="/design/rule-sets/{r.id}" class="group block">
							<span class="flex items-center gap-1.5 font-medium group-hover:underline"
								>{r.slug}{#if r.kind === 'global'}<Tag
										><Lock class="size-3" aria-label="Cannot be deleted" />global</Tag
									>{/if}</span
							>
							<span class="block truncate text-[12.5px] text-muted-foreground">{r.description}</span
							>
						</a>
					</Table.Cell>
					<Table.Cell class="tnum hidden text-right sm:table-cell">{r.entries.length}</Table.Cell>
					<Table.Cell class="hidden md:table-cell">
						<span class="block truncate text-[13px]">{r.usedBy[0]?.label ?? 'Not used'}</span>
						{#if r.usedBy.length > 1}<span class="text-[12px] text-muted-foreground"
								>and {r.usedBy.length - 1} more</span
							>{/if}
					</Table.Cell>
					<Table.Cell>
						{#if r.warnings.length}<span
								class="inline-flex items-center gap-1.5 text-[12.5px] text-warn-text"
								><TriangleAlert class="size-3.5" />{r.warnings.length}</span
							>{:else}<span class="text-muted-foreground">—</span>{/if}
					</Table.Cell>
				</Table.Row>
			{/each}
		</Table.Body>
	</Table.Root>
</div>
<p class="mt-3 text-[12.5px] text-muted-foreground">
	Every HTTP service also has its own rule set with a fallback; you edit it on the service’s Access
	tab.
</p>
