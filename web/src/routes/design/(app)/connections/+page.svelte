<script lang="ts">
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import StatusBadge from '$lib/components/hashi/StatusBadge.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import { Button } from '$lib/components/ui/button';
	import { connections } from '$lib/design/mock';
	import { Plus, MoreHorizontal } from '@lucide/svelte';

	const groups = Object.entries(Object.groupBy(connections, (c) => c.type));
</script>

<PageHeader
	title="Connections"
	description="The outside systems Hashi talks to. Credentials are kept in the vault and are never shown again after saving."
>
	{#snippet actions()}<Button><Plus />Add connection</Button>{/snippet}
</PageHeader>
<div class="divide-y rounded-2xl border bg-card">
	{#each groups as [type, list] (type)}
		<section class="px-4 py-3">
			<h2 class="mb-1 text-[12.5px] font-medium text-muted-foreground">{type}</h2>
			<ul>
				{#each list ?? [] as c (c.name)}
					<li class="flex flex-wrap items-center gap-3 py-2 text-[13px]">
						<span class="min-w-40 font-medium">{c.name}</span>
						<span class="min-w-0 flex-1 truncate text-muted-foreground">{c.detail}</span>
						{#if c.required}<Tag>required</Tag>{/if}
						<span class="hidden w-36 text-right text-[12.5px] text-muted-foreground md:inline"
							>checked {c.validated}</span
						>
						<span class="w-24"><StatusBadge state={c.health} /></span>
						<Button variant="ghost" size="icon-sm" aria-label="Actions for {c.name}"
							><MoreHorizontal /></Button
						>
					</li>
				{/each}
			</ul>
		</section>
	{/each}
</div>
<p class="mt-2 text-[12.5px] text-muted-foreground">
	Hashi needs at least one DNS provider, one Traefik host and one firewall host; those cannot be
	removed while they are the last of their kind.
</p>
