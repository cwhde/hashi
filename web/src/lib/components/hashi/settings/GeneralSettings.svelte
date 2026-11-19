<script lang="ts">
	// Settings → General (09 §2): domains, the internal address, reconcile interval and how times show.
	import { onMount } from 'svelte';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import SaveBar from './SaveBar.svelte';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	import * as ToggleGroup from '#lib/components/ui/toggle-group/index.ts';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { withReauth } from '#lib/auth/reauth.ts';
	import { setTimeDisplay } from '#lib/format.ts';
	import { Draft, guardUnsaved } from '#lib/unsaved.svelte.ts';
	import { Saver } from '#lib/settings/saver.svelte.ts';

	let g = $state<Schemas['GeneralSettings'] | null>(null);
	let loadError = $state('');
	const draft = new Draft(() => g);
	const saver = new Saver();
	guardUnsaved(() => draft.dirty);

	onMount(async () => {
		const r = await api.GET('/api/settings/general');
		if (!r.data) loadError = problemText(r.error);
		else {
			g = r.data;
			draft.mark();
		}
	});
	const save = () =>
		saver.run(
			async () => {
				// Trusted proxies share this record but are edited under Security: send the stored ones back.
				const cur = await api.GET('/api/settings/general');
				if (!cur.data) throw cur.error;
				const body = { ...g!, trusted_proxies: cur.data.trusted_proxies };
				g = await withReauth(() => api.PUT('/api/settings/general', { body }));
				setTimeDisplay(g.time_display ?? 'local');
			},
			'Saved.',
			draft
		);
</script>

{#if loadError}<p role="alert" class="text-md text-bad-text">{loadError}</p>
{:else if !g}<Skeleton class="h-40 rounded-2xl" />
{:else}
	<Panel title="Domains and addresses">
		<div class="grid gap-4 md:grid-cols-2">
			<div class="space-y-1.5">
				<Label for="rd">Root domain</Label><Input id="rd" value={g.root_domain} disabled />
				<p class="text-xs text-muted-foreground">
					Fixed after setup: your passkeys are bound to it.
				</p>
			</div>
			<div class="space-y-1.5">
				<Label for="ad">Admin domain</Label><Input id="ad" bind:value={g.admin_domain} />
			</div>
			<div class="space-y-1.5">
				<Label for="iu">Internal Hashi URL</Label><Input
					id="iu"
					bind:value={g.internal_url}
					class="font-mono"
				/>
			</div>
			<div class="space-y-1.5">
				<Label for="si">Reconcile every (minutes)</Label><Input
					id="si"
					type="number"
					min="1"
					max="1440"
					value={g.sync_interval_seconds / 60}
					oninput={(e) =>
						(g!.sync_interval_seconds = Math.round(Number(e.currentTarget.value) * 60))}
				/>
			</div>
		</div>
	</Panel>
	<Panel title="Times">
		<ToggleGroup.Root
			type="single"
			variant="outline"
			value={g.time_display ?? 'local'}
			onValueChange={(v) => v && (g!.time_display = v as 'local' | 'utc')}
			aria-label="Show times in"
		>
			<ToggleGroup.Item value="local">Local time</ToggleGroup.Item>
			<ToggleGroup.Item value="utc">UTC</ToggleGroup.Item>
		</ToggleGroup.Root>
	</Panel>
	<SaveBar {saver} dirty={draft.dirty} onsave={save} />
{/if}
