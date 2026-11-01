<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import * as Sheet from '$lib/components/ui/sheet';
	import * as Tabs from '$lib/components/ui/tabs';
	import { CircleAlert, Plus } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { withReauth } from '$lib/auth/reauth';
	import { dateTime } from '$lib/format';

	// Security for S9: who may sign in at the edge, and the country database (06 §7, §12). The dashboard, counters, CAPTCHA
	// and engine tabs arrive with the abuse engine.
	type Provider = Schemas['SSOProvider'];
	let data = $state<Schemas['SSOProviders'] | null>(null);
	let geo = $state<Schemas['GeoIPStatus'] | null>(null);
	let error = $state('');
	let editing = $state<{ id: string; form: Schemas['SSOProviderInput'] } | null>(null);
	let formError = $state('');
	let busy = $state(false);
	let geoForm = $state({ account_id: '', license_key: '', url: '' });
	let geoError = $state('');

	let limits = $state<Schemas['EdgeSSOSettings'] | null>(null);
	let limitsNote = $state('');
	async function saveLimits() {
		if (!limits) return;
		const r = await api.PUT('/api/settings/edge-sso', { body: limits });
		limitsNote = r.error ? problemText(r.error) : 'Saved.';
	}
	async function load() {
		const [p, g, l] = await Promise.all([
			api.GET('/api/security/sso/providers'),
			api.GET('/api/security/geoip'),
			api.GET('/api/settings/edge-sso')
		]);
		limits = l.data ?? null;
		if (p.error || g.error) {
			error = problemText(p.error ?? g.error);
			return;
		}
		data = p.data;
		geo = g.data;
		geoForm = { account_id: g.data.account_id, license_key: '', url: g.data.url };
	}
	onMount(load);

	const blank = (first: boolean): Schemas['SSOProviderInput'] => ({
		name: '',
		issuer: '',
		client_id: '',
		client_secret: '',
		scopes: 'openid profile email groups',
		username_claim: 'preferred_username',
		email_claim: 'email',
		groups_claim: 'groups',
		is_default: first
	});
	const fromProvider = (p: Provider): Schemas['SSOProviderInput'] => ({
		name: p.name,
		issuer: p.issuer,
		client_id: p.client_id,
		client_secret: '',
		scopes: p.scopes,
		username_claim: p.username_claim,
		email_claim: p.email_claim,
		groups_claim: p.groups_claim,
		is_default: p.is_default
	});
	async function saveProvider() {
		if (!editing) return;
		const { id, form } = editing;
		busy = true;
		formError = '';
		try {
			await withReauth(() =>
				id
					? api.PUT('/api/security/sso/providers/{id}', { params: { path: { id } }, body: form })
					: api.POST('/api/security/sso/providers', { body: form })
			);
			editing = null;
			await load();
		} catch (e) {
			formError = problemText(e);
		} finally {
			busy = false;
		}
	}
	async function removeProvider(id: string) {
		formError = '';
		try {
			await withReauth(() =>
				api.DELETE('/api/security/sso/providers/{id}', { params: { path: { id } } })
			);
			editing = null;
			await load();
		} catch (e) {
			formError = problemText(e);
		}
	}
	async function saveGeo() {
		busy = true;
		geoError = '';
		try {
			geo = await withReauth(() => api.PUT('/api/security/geoip', { body: geoForm }));
			geoError = geo?.error ?? '';
			geoForm.license_key = '';
		} catch (e) {
			geoError = problemText(e);
		} finally {
			busy = false;
		}
	}
</script>

<PageHeader
	title="Security"
	description="Who can sign in at the edge, and where countries come from."
/>

{#if error}
	<p class="flex items-center gap-2 text-[13px] text-bad-text">
		<CircleAlert class="size-3.5" />{error}
	</p>
{/if}

<Tabs.Root value="sso">
	<Tabs.List class="mb-4">
		<Tabs.Trigger value="sso">Sign-in providers</Tabs.Trigger>
		<Tabs.Trigger value="limits">Sessions</Tabs.Trigger>
		<Tabs.Trigger value="geo">GeoIP</Tabs.Trigger>
	</Tabs.List>

	<Tabs.Content value="sso" class="space-y-3">
		<Panel
			flush
			title="Sign-in providers"
			description={data
				? `One login on .${data.cookie_domain} covers every protected subdomain`
				: ''}
		>
			{#snippet actions()}<Button
					size="sm"
					variant="outline"
					onclick={() => (editing = { id: '', form: blank(!data?.providers.length) })}
					><Plus />Add provider</Button
				>{/snippet}
			<ul class="divide-y text-[13px]">
				{#each data?.providers ?? [] as p (p.id)}
					<li class="flex items-center gap-3 px-4 py-3">
						<button
							class="flex-1 text-left"
							onclick={() => (editing = { id: p.id, form: fromProvider(p) })}
						>
							<span class="font-medium hover:underline">{p.name}</span>
							{#if p.is_default}<Tag>default</Tag>{/if}
							<span class="block text-muted-foreground">{p.issuer}</span>
							{#if p.discovery_error}<span class="block text-[12.5px] text-bad-text"
									>{p.discovery_error}</span
								>{/if}
						</button>
						<Tag tone={p.discovery_error ? 'bad' : 'ok'}
							>{p.discovery_error ? 'discovery failing' : 'reachable'}</Tag
						>
					</li>
				{:else}
					<li class="px-4 py-3 text-muted-foreground">
						No provider yet. Rules that ask for sign-in refuse everyone until there is one.
					</li>
				{/each}
			</ul>
		</Panel>
		{#if data}<p class="text-[12.5px] text-muted-foreground">
				Register this address at the provider: <Mono>{data.callback_url}</Mono>
			</p>{/if}
	</Tabs.Content>

	<Tabs.Content value="limits">
		{#if limits}
			<Panel title="Sessions" description="How long a sign-in at the edge lasts.">
				<div class="grid max-w-lg gap-3 sm:grid-cols-2">
					<div class="space-y-1.5">
						<Label for="lm">Longest session (hours)</Label>
						<Input id="lm" type="number" bind:value={limits.max_session_hours} />
					</div>
					<div class="space-y-1.5">
						<Label for="li">Ends when unused for (hours)</Label>
						<Input id="li" type="number" bind:value={limits.idle_timeout_hours} />
					</div>
				</div>
				<label class="mt-3 flex items-center gap-2 text-[13px]"
					><Switch bind:checked={limits.remember_device} />Stay signed in after closing the browser</label
				>
				<div class="mt-3 flex items-center gap-3">
					<Button size="sm" onclick={saveLimits}>Save sessions</Button>
					<span class="text-[13px] text-muted-foreground">{limitsNote}</span>
				</div>
			</Panel>
		{/if}
	</Tabs.Content>

	<Tabs.Content value="geo" class="grid gap-4 lg:grid-cols-2">
		<Panel
			title="Databases"
			description="Used by rules and firewall sets that name a country or network."
		>
			<ul class="space-y-2 text-[13px]">
				{#each geo?.databases ?? [] as d (d.edition)}
					<li class="flex items-center gap-2">
						<span class="flex-1"
							><span class="font-medium">{d.edition}</span><span
								class="block text-[12.5px] text-muted-foreground"
								>built {dateTime(d.built_at)}, fetched {dateTime(d.fetched_at)}</span
							></span
						>
						<Tag tone={d.stale ? 'warn' : 'ok'}>{d.stale ? 'over 14 days old' : 'current'}</Tag>
					</li>
				{:else}
					<li class="text-muted-foreground">
						GeoIP unavailable: rules with countries match nobody.
					</li>
				{/each}
			</ul>
		</Panel>
		<Panel
			title="Download source"
			description="A MaxMind account, or an address with {'{edition}'} in it."
		>
			<div class="space-y-3">
				<div class="grid gap-3 sm:grid-cols-2">
					<div class="space-y-1.5">
						<Label for="ga">MaxMind account ID</Label><Input
							id="ga"
							bind:value={geoForm.account_id}
						/>
					</div>
					<div class="space-y-1.5">
						<Label for="gk">Licence key</Label><Input
							id="gk"
							type="password"
							bind:value={geoForm.license_key}
							placeholder={geo?.has_key ? 'Stored' : ''}
						/>
					</div>
				</div>
				<div class="space-y-1.5">
					<Label for="gu">Or download address</Label><Input
						id="gu"
						class="font-mono"
						bind:value={geoForm.url}
						placeholder="https://mirror.example.com/{'{edition}'}.tar.gz"
					/>
				</div>
				{#if geoError}<p class="text-[13px] text-bad-text">{geoError}</p>{/if}
				<Button size="sm" disabled={busy} onclick={saveGeo}>Save and download</Button>
			</div>
		</Panel>
	</Tabs.Content>
</Tabs.Root>

<Sheet.Root open={!!editing} onOpenChange={(o) => !o && (editing = null)}>
	<Sheet.Content class="w-full sm:max-w-lg">
		<Sheet.Header>
			<Sheet.Title>{editing?.id ? 'Sign-in provider' : 'New sign-in provider'}</Sheet.Title>
			<Sheet.Description
				>Hashi reads the provider's discovery document before saving.</Sheet.Description
			>
		</Sheet.Header>
		{#if editing}
			<div class="space-y-3 overflow-y-auto px-4 pb-4">
				<div class="space-y-1.5">
					<Label for="pn">Name</Label><Input id="pn" bind:value={editing.form.name} />
				</div>
				<div class="space-y-1.5">
					<Label for="pi">Issuer</Label><Input
						id="pi"
						class="font-mono"
						placeholder="https://sso.example.com"
						bind:value={editing.form.issuer}
					/>
				</div>
				<div class="grid gap-3 sm:grid-cols-2">
					<div class="space-y-1.5">
						<Label for="pc">Client ID</Label><Input
							id="pc"
							class="font-mono"
							bind:value={editing.form.client_id}
						/>
					</div>
					<div class="space-y-1.5">
						<Label for="ps">Client secret</Label><Input
							id="ps"
							type="password"
							placeholder={editing.id ? 'Stored' : ''}
							bind:value={editing.form.client_secret}
						/>
					</div>
				</div>
				<div class="space-y-1.5">
					<Label for="pp">Scopes</Label><Input
						id="pp"
						class="font-mono"
						bind:value={editing.form.scopes}
					/>
				</div>
				<div class="grid gap-3 sm:grid-cols-3">
					<div class="space-y-1.5">
						<Label for="pu">Username claim</Label><Input
							id="pu"
							class="font-mono"
							bind:value={editing.form.username_claim}
						/>
					</div>
					<div class="space-y-1.5">
						<Label for="pe">Email claim</Label><Input
							id="pe"
							class="font-mono"
							bind:value={editing.form.email_claim}
						/>
					</div>
					<div class="space-y-1.5">
						<Label for="pg">Groups claim</Label><Input
							id="pg"
							class="font-mono"
							bind:value={editing.form.groups_claim}
						/>
					</div>
				</div>
				<label class="flex items-center gap-2 text-[13px]"
					><Switch bind:checked={editing.form.is_default} />Default provider</label
				>
				{#if formError}<p class="text-[13px] text-bad-text">{formError}</p>{/if}
			</div>
		{/if}
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			{#if editing?.id}<Button
					variant="ghost"
					class="mr-auto text-bad-text"
					onclick={() => editing && removeProvider(editing.id)}>Delete</Button
				>{/if}
			<Button variant="ghost" onclick={() => (editing = null)}>Cancel</Button>
			<Button disabled={busy} onclick={saveProvider}>Save provider</Button>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>
