<script lang="ts">
	import { onMount } from 'svelte';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import Mono from '#lib/components/hashi/Mono.svelte';
	import Tag from '#lib/components/hashi/Tag.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';

	// 03 §9, 10b §3: agents' internal names. DNS only: no service exists at these names.
	let names = $state<Schemas['AgentDNSPreview']['rewrites']>([]);
	let settings = $state<Schemas['AgentDNSSettings'] | null>(null);
	let error = $state('');
	let q = $state(''); // bug 64: the same filter Records has
	const shown = $derived(
		names.filter((a) =>
			[a.domain, a.agent, a.answer].join(' ').toLowerCase().includes(q.trim().toLowerCase())
		)
	);
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

{#if error}<p role="alert" class="mb-3 text-md text-bad-text">{error}</p>{/if}
<Panel
	title={settings?.domain ?? 'Internal agent names'}
	description="DNS only, not reverse proxy: stable internal names for Pulse agents"
>
	{#snippet actions()}<Button size="sm" variant="outline" href="/settings?category=agent-dns"
			>Settings</Button
		>{/snippet}
	{#if settings && !settings.enabled}
		<p class="mb-2 text-md text-muted-foreground">Turned off in settings; nothing is written.</p>
	{/if}
	{#if names.length === 0}<p class="text-md text-muted-foreground">No agents yet.</p>{/if}
	{#if names.length > 5}<Input
			bind:value={q}
			placeholder="Filter by name or address"
			aria-label="Filter agent names"
			class="mb-2 max-w-72"
		/>{/if}
	<ul class="divide-y text-md">
		{#each shown as a (a.agent_id)}
			<li class="flex flex-wrap items-center gap-3 py-2">
				<Mono class="min-w-0 flex-1 truncate">{a.domain || a.agent}</Mono>
				<Mono class="text-muted-foreground">{a.answer}</Mono>
				{#if a.collision}<Tag tone="warn">name taken</Tag>
				{:else if a.status === 'stale'}<Tag tone="warn">stale · kept</Tag>
				{:else if a.status === 'fresh'}<Tag tone="ok">fresh</Tag>
				{:else}<Tag>{a.status}</Tag>{/if}
				{#if a.problem}<span class="w-full text-xs text-muted-foreground">{a.problem}</span>{/if}
			</li>
		{/each}
	</ul>
</Panel>
