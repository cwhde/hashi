<script lang="ts">
	import { onMount } from 'svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import StatusBadge from '$lib/components/hashi/StatusBadge.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import { Button } from '$lib/components/ui/button';
	import { api, problemText, type Schemas } from '$lib/api/client';

	// 10b §3: each AdGuard instance's rewrites and what Hashi would change there. Reading changes nothing.
	type One = {
		conn: Schemas['AdGuardConnection'];
		data?: Schemas['AdGuardRewrites'];
		error?: string;
	};
	let items = $state<One[]>([]);
	let error = $state('');
	let loaded = $state(false);
	onMount(async () => {
		const c = await api.GET('/api/adguard/connections');
		loaded = true;
		if (!c.data) {
			error = problemText(c.error);
			return;
		}
		items = c.data.connections.filter((x) => x.mode !== 'disabled').map((conn) => ({ conn }));
		await Promise.all(
			items.map(async (it, i) => {
				const r = await api.GET('/api/adguard/connections/{id}/rewrites', {
					params: { path: { id: it.conn.id } }
				});
				if (r.data) items[i]!.data = r.data;
				else items[i]!.error = problemText(r.error);
			})
		);
	});
	const actionText: Record<string, string> = {
		create: 'Add',
		update: 'Change',
		delete: 'Remove',
		conflict: 'Left alone'
	};
	const label = (it: One) => {
		const changes = it.data?.planned.filter((p) => p.action !== 'conflict').length ?? 0;
		if (it.conn.mode === 'read_only') return 'Read-only';
		return changes ? `${changes} to change` : 'In sync';
	};
</script>

{#if error}<p role="alert" class="text-[13px] text-bad-text">{error}</p>{/if}
{#if loaded && items.length === 0 && !error}
	<Panel title="No AdGuard instance">
		<p class="text-[13px] text-muted-foreground">Add one to manage internal rewrites.</p>
		<Button class="mt-3" size="sm" variant="outline" href="/connections">Go to Connections</Button>
	</Panel>
{/if}
<div class="grid gap-4 lg:grid-cols-2">
	{#each items as it (it.conn.id)}
		<Panel
			title={it.conn.name}
			description={it.data
				? `${it.data.rewrites.length} rewrites, ${it.data.rewrites.filter((r) => r.owned).length} owned by Hashi`
				: ''}
		>
			{#snippet actions()}<StatusBadge
					state={it.error ? 'down' : it.conn.mode === 'read_only' ? 'unknown' : 'up'}
					label={it.error ? 'Unreachable' : label(it)}
				/>{/snippet}
			{#if it.error}<p role="alert" class="text-[13px] text-bad-text">{it.error}</p>{/if}
			{#if it.data}
				<ul class="space-y-1.5 text-[13px]">
					{#each it.data.rewrites as r, i (i)}
						<li class="flex justify-between gap-3">
							<Mono class="min-w-0 truncate">{r.domain}</Mono>
							<span class="flex items-center gap-2"
								><Mono class="text-muted-foreground">{r.answer}</Mono>{#if !r.owned}<Tag
										>manual · untouched</Tag
									>{/if}</span
							>
						</li>
					{/each}
				</ul>
				{#if it.data.planned.length}
					<h3 class="mt-4 mb-1 text-[12.5px] font-medium text-muted-foreground">
						{it.conn.mode === 'read_only' ? 'Would change if active' : 'Waiting to change'}
					</h3>
					<ul class="space-y-1.5 text-[13px]">
						{#each it.data.planned as p, i (i)}
							<li class="flex flex-wrap items-center gap-2">
								<Tag tone={p.action === 'conflict' ? 'warn' : 'neutral'}
									>{actionText[p.action] ?? p.action}</Tag
								><Mono class="min-w-0 flex-1 truncate">{p.domain}</Mono><Mono
									class="text-muted-foreground">{p.answer ?? ''}</Mono
								>
								{#if p.note}<span class="w-full text-[12px] text-muted-foreground">{p.note}</span
									>{/if}
							</li>
						{/each}
					</ul>
				{/if}
			{/if}
		</Panel>
	{/each}
</div>
