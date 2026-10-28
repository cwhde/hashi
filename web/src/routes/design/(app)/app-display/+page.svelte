<script lang="ts">
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Switch } from '$lib/components/ui/switch';
	import { services, monitors } from '$lib/design/mock';
	import { GripVertical, ExternalLink } from '@lucide/svelte';
</script>

<PageHeader
	title="App display"
	description="What visitors see on the public app dashboard and status page. Both run on their own ports with no admin access."
>
	{#snippet actions()}
		<Button variant="outline" size="sm" href="/design/public-apps"
			><ExternalLink />Open dashboard</Button
		>
		<Button variant="outline" size="sm" href="/design/public-status"
			><ExternalLink />Open status page</Button
		>
	{/snippet}
</PageHeader>
<div class="grid gap-4 lg:grid-cols-2">
	<Panel
		title="App dashboard"
		description="Port 8081 · {services.filter((r) => r.dashboard).length} apps · sorted by group"
		flush
	>
		{#snippet actions()}<Switch checked aria-label="Public dashboard enabled" />{/snippet}
		<ul class="divide-y">
			{#each services.filter((r) => r.origin !== 'system') as r (r.id)}
				<li class="flex items-center gap-3 px-4 py-2 text-[13px]">
					<GripVertical class="size-4 text-muted-foreground" />
					<span
						class="grid size-7 place-items-center rounded-lg border bg-surface-2 text-[12px] font-semibold"
						>{r.name[0]}</span
					>
					<span class="flex-1"
						><span class="font-medium">{r.name}</span>
						<span class="text-muted-foreground">· {r.group}</span></span
					>
					<Switch checked={r.dashboard} aria-label="Show {r.name}" />
				</li>
			{/each}
		</ul>
	</Panel>
	<Panel
		title="Status page"
		description="Port 8082 · names, groups, state and history only; never hosts, IPs or errors"
		flush
	>
		{#snippet actions()}<Switch checked aria-label="Public status enabled" />{/snippet}
		<ul class="divide-y">
			{#each monitors.slice(0, 9) as m (m.id)}
				<li class="flex items-center gap-3 px-4 py-2 text-[13px]">
					<span class="flex-1">{m.name} <span class="text-muted-foreground">· {m.group}</span></span
					>{#if m.public}<Tag tone="ok">public</Tag>{:else}<Tag>hidden</Tag>{/if}
				</li>
			{/each}
		</ul>
	</Panel>
</div>
