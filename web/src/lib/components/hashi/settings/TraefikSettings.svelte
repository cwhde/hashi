<script lang="ts">
	// Settings → Services and Traefik (09 §2, 10c §1.2): which of your middlewares every user HTTP service gets. The
	// names come from the file on the Traefik page; saving sends that file back unchanged.
	import { onMount } from 'svelte';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import Mono from '#lib/components/hashi/Mono.svelte';
	import Tag from '#lib/components/hashi/Tag.svelte';
	import SaveBar from './SaveBar.svelte';
	import { Switch } from '#lib/components/ui/switch/index.ts';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { saveText } from '#lib/services.ts';
	import { pendingPlans } from '#lib/sync-pending.svelte.ts';
	import { Draft, guardUnsaved } from '#lib/unsaved.svelte.ts';
	import { Saver } from '#lib/settings/saver.svelte.ts';

	let file = $state<Schemas['UserMiddlewares'] | null>(null);
	let every = $state<string[]>([]);
	let error = $state('');
	const draft = new Draft(() => [...every].sort());
	const saver = new Saver();
	guardUnsaved(() => draft.dirty);
	onMount(async () => {
		const r = await api.GET('/api/traefik/user-middlewares');
		if (!r.data) error = problemText(r.error);
		else {
			file = r.data;
			every = [...r.data.every_service];
			draft.mark();
		}
	});
	const toggle = (name: string, on: boolean) =>
		(every = on ? [...every.filter((n) => n !== name), name] : every.filter((n) => n !== name));
	const save = () =>
		saver.run(
			async () => {
				const r = await api.PUT('/api/traefik/user-middlewares', {
					body: { yaml: file!.yaml, every_service: every }
				});
				if (!r.data) throw r.error;
				file = r.data.file;
				every = [...r.data.file.every_service];
				const t = saveText(r.data);
				saver.note = { ok: !t.bad, msg: t.text };
				await pendingPlans.refresh();
			},
			'',
			draft
		);
</script>

<Panel title="Middlewares on every service" flush>
	{#if error}<p role="alert" class="px-4 py-3 text-md text-bad-text">{error}</p>
	{:else if file && file.middlewares.length === 0}
		<div class="flex items-center gap-3 px-4 py-3 text-md text-muted-foreground">
			None defined yet.<Button size="sm" variant="outline" href="/traefik?tab=middlewares"
				>Write one</Button
			>
		</div>
	{:else if file}
		<ul class="divide-y text-md">
			{#each file.middlewares as m (m.name)}
				<li class="flex items-center gap-3 px-4 py-2.5">
					<span class="flex-1"><Mono>{m.name}</Mono> <Tag>{m.type}</Tag></span>
					<Switch
						checked={every.includes(m.name)}
						onCheckedChange={(on) => toggle(m.name, on)}
						aria-label="Use {m.name} on every service"
					/>
				</li>
			{/each}
		</ul>
		<div class="px-4 py-3"><SaveBar {saver} dirty={draft.dirty} onsave={save} /></div>
	{/if}
</Panel>
