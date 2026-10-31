<script lang="ts">
	import { onMount } from 'svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import { Button } from '$lib/components/ui/button';
	import { api, problemText, type Schemas } from '$lib/api/client';

	// 03 §9, 10b §3: agents' internal names. DNS only: no service exists at these names.
	let names = $state<Schemas['AgentDNSPreview']['rewrites']>([]);
	let settings = $state<Schemas['AgentDNSSettings'] | null>(null);
	let error = $state('');
	onMount(async () => {
		const [n, s] = await Promise.all([
			api.POST('/api/settings/internal-agent-dns/preview-sync'),
			api.GET('/api/settings/internal-agent-dns')
		]);
		if (n.data) names = n.data.rewrites;
		else error = problemText(n.error);
		if (s.data) settings = s.data;
	});
</script>

{#if error}<p role="alert" class="mb-3 text-[13px] text-bad-text">{error}</p>{/if}
<Panel
	title={settings?.domain ?? 'Internal agent names'}
	description="DNS only, not reverse proxy: stable internal names for Pulse agents"
>
	{#snippet actions()}<Button size="sm" variant="outline" href="/settings?category=agent-dns"
			>Settings</Button
		>{/snippet}
	{#if settings && !settings.enabled}
		<p class="mb-2 text-[13px] text-muted-foreground">
			Turned off in settings; nothing is written.
		</p>
	{/if}
	{#if names.length === 0}<p class="text-[13px] text-muted-foreground">No agents yet.</p>{/if}
	<ul class="divide-y text-[13px]">
		{#each names as a (a.agent_id)}
			<li class="flex flex-wrap items-center gap-3 py-2">
				<Mono class="min-w-0 flex-1 truncate">{a.domain || a.agent}</Mono>
				<Mono class="text-muted-foreground">{a.answer}</Mono>
				{#if a.collision}<Tag tone="warn">name taken</Tag>
				{:else if a.status === 'stale'}<Tag tone="warn">stale · kept</Tag>
				{:else if a.status === 'fresh'}<Tag tone="ok">fresh</Tag>
				{:else}<Tag>{a.status}</Tag>{/if}
				{#if a.problem}<span class="w-full text-[12px] text-muted-foreground">{a.problem}</span
					>{/if}
			</li>
		{/each}
	</ul>
</Panel>
