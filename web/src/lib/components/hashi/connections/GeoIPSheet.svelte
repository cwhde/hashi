<script lang="ts">
	// Where the GeoIP databases come from (06 §12): a MaxMind account or a download address. A connection (10c §1.2).
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import * as Sheet from '#lib/components/ui/sheet/index.ts';
	import Tag from '#lib/components/hashi/Tag.svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { withReauth } from '#lib/auth/reauth.ts';
	import { dateTime } from '#lib/format.ts';

	let {
		open = $bindable(false),
		geo,
		onsaved
	}: { open?: boolean; geo: Schemas['GeoIPStatus'] | null; onsaved: () => void } = $props();

	let f = $state({ account_id: '', license_key: '', url: '' });
	let error = $state('');
	let busy = $state(false);
	$effect(() => {
		if (!open) return;
		error = '';
		f = { account_id: geo?.account_id ?? '', license_key: '', url: geo?.url ?? '' };
	});
	async function save() {
		busy = true;
		error = '';
		try {
			const g = await withReauth(() => api.PUT('/api/security/geoip', { body: f }));
			if (g.error) error = g.error;
			else open = false;
			onsaved();
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = false;
		}
	}
</script>

<Sheet.Root bind:open>
	<Sheet.Content class="w-full sm:max-w-lg">
		<Sheet.Header>
			<Sheet.Title>GeoIP database</Sheet.Title>
		</Sheet.Header>
		<div class="space-y-3 overflow-y-auto px-4 pb-4 text-md">
			{#if geo?.databases.length}
				<ul class="space-y-2">
					{#each geo.databases as d (d.edition)}
						<li class="flex items-center gap-2">
							<span class="flex-1"
								><span class="font-medium">{d.edition}</span><span
									class="block text-sm text-muted-foreground"
									>built {dateTime(d.built_at)}, fetched {dateTime(d.fetched_at)}</span
								></span
							>
							<Tag tone={d.stale ? 'warn' : 'ok'}>{d.stale ? 'over 14 days old' : 'current'}</Tag>
						</li>
					{/each}
				</ul>
			{/if}
			<div class="grid gap-3 sm:grid-cols-2">
				<div class="space-y-1.5">
					<Label for="ga">MaxMind account ID</Label><Input id="ga" bind:value={f.account_id} />
				</div>
				<div class="space-y-1.5">
					<Label for="gk">Licence key</Label><Input
						id="gk"
						type="password"
						bind:value={f.license_key}
						placeholder={geo?.has_key ? 'Stored' : ''}
					/>
				</div>
			</div>
			<div class="space-y-1.5">
				<Label for="gu">Or download address</Label><Input
					id="gu"
					class="font-mono"
					bind:value={f.url}
					placeholder="https://mirror.example.com/{'{edition}'}.tar.gz"
				/>
			</div>
			{#if error}<p role="alert" class="text-bad-text">{error}</p>{/if}
		</div>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			<Button variant="outline" onclick={() => (open = false)}>Cancel</Button>
			<Button disabled={busy} onclick={save}>Save and download</Button>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>
