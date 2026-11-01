<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Table from '$lib/components/ui/table';
	import * as Sheet from '$lib/components/ui/sheet';
	import { CircleAlert, Lock, Plus, TriangleAlert } from '@lucide/svelte';
	import { api, problemText } from '$lib/api/client';
	import { listRuleSets, type RuleSet } from '$lib/rules';

	let sets = $state<RuleSet[] | null>(null);
	let error = $state('');
	let adding = $state(false);
	let form = $state({ slug: '', name: '', description: '' });
	let addError = $state('');

	onMount(async () => {
		try {
			sets = await listRuleSets();
		} catch (e) {
			error = problemText(e);
		}
	});
	async function add() {
		addError = '';
		const r = await api.POST('/api/rule-sets', { body: { ...form, name: form.name || form.slug } });
		if (r.error) {
			addError = problemText(r.error);
			return;
		}
		await goto(`/rule-sets/${r.data.id}`);
	}
</script>

<PageHeader
	title="Rule sets"
	description="Reusable lists of rules. A service runs its own rules first, then the global set, then its fallback. The first rule that matches decides."
>
	{#snippet actions()}<Button onclick={() => (adding = true)}><Plus />Add rule set</Button
		>{/snippet}
</PageHeader>

{#if error}
	<p class="flex items-center gap-2 text-[13px] text-bad-text">
		<CircleAlert class="size-3.5" />{error}
	</p>
{:else if !sets}
	<Skeleton class="h-40 w-full rounded-2xl" />
{:else}
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
				{#each sets as r (r.id)}
					<Table.Row>
						<Table.Cell class="max-w-[30rem] pl-4">
							<a href="/rule-sets/{r.id}" class="group block">
								<span class="flex items-center gap-1.5 font-medium group-hover:underline"
									>{r.slug}{#if r.kind === 'global'}<Tag
											><Lock class="size-3" aria-label="Cannot be deleted" />global</Tag
										>{/if}</span
								>
								<span class="block truncate text-[12.5px] text-muted-foreground"
									>{r.description}</span
								>
							</a>
						</Table.Cell>
						<Table.Cell class="tnum hidden text-right sm:table-cell">{r.entries.length}</Table.Cell>
						<Table.Cell class="hidden md:table-cell">
							<span class="block truncate text-[13px]"
								>{r.kind === 'global' ? 'Every service' : (r.used_by[0]?.label ?? 'Not used')}</span
							>
							{#if r.used_by.length > 1}<span class="text-[12px] text-muted-foreground"
									>and {r.used_by.length - 1} more</span
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
		Every HTTP service also has its own rule set with a fallback; you edit it on the service's
		Access tab.
	</p>
{/if}

<Sheet.Root bind:open={adding}>
	<Sheet.Content class="w-full sm:max-w-md">
		<Sheet.Header><Sheet.Title>New rule set</Sheet.Title></Sheet.Header>
		<div class="space-y-3 px-4">
			<div class="space-y-1.5">
				<Label for="sl">Slug</Label><Input
					id="sl"
					class="font-mono"
					bind:value={form.slug}
					placeholder="admin-protect"
				/>
			</div>
			<div class="space-y-1.5">
				<Label for="nm">Name</Label><Input id="nm" bind:value={form.name} />
			</div>
			<div class="space-y-1.5">
				<Label for="ds">Description</Label><Input id="ds" bind:value={form.description} />
			</div>
			{#if addError}<p class="text-[13px] text-bad-text">{addError}</p>{/if}
		</div>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			<Button variant="ghost" onclick={() => (adding = false)}>Cancel</Button>
			<Button disabled={!form.slug} onclick={add}>Add rule set</Button>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>
