<script lang="ts">
	// Add or edit an edge sign-in provider (06 §7): an OIDC issuer Hashi reaches with a client secret, so it is a
	// connection (10c §1.2). Hashi reads the provider's discovery document before saving.
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import { Switch } from '#lib/components/ui/switch/index.ts';
	import * as Sheet from '#lib/components/ui/sheet/index.ts';
	import MoreOptions from '#lib/components/hashi/MoreOptions.svelte';
	import Mono from '#lib/components/hashi/Mono.svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { withReauth } from '#lib/auth/reauth.ts';

	type Provider = Schemas['SSOProvider'];
	let {
		open = $bindable(false),
		provider = null,
		first = false,
		callback = '',
		onsaved
	}: {
		open?: boolean;
		provider?: Provider | null;
		first?: boolean;
		callback?: string;
		onsaved: () => void;
	} = $props();

	const defaults = {
		scopes: 'openid profile email groups',
		username_claim: 'preferred_username',
		email_claim: 'email',
		groups_claim: 'groups'
	};
	let f = $state<Schemas['SSOProviderInput']>({
		name: '',
		issuer: '',
		client_id: '',
		client_secret: '',
		...defaults,
		is_default: false
	});
	let error = $state('');
	let busy = $state(false);
	$effect(() => {
		if (!open) return;
		error = '';
		f = provider
			? {
					name: provider.name,
					issuer: provider.issuer,
					client_id: provider.client_id,
					client_secret: '',
					scopes: provider.scopes,
					username_claim: provider.username_claim,
					email_claim: provider.email_claim,
					groups_claim: provider.groups_claim,
					is_default: provider.is_default
				}
			: { name: '', issuer: '', client_id: '', client_secret: '', ...defaults, is_default: first };
	});
	// 10c §3.1: the closed section says when something inside differs from the default.
	const changed = $derived(
		(Object.keys(defaults) as (keyof typeof defaults)[]).filter((k) => f[k] !== defaults[k]).length
	);

	async function save() {
		busy = true;
		error = '';
		try {
			const id = provider?.id;
			await withReauth(() =>
				id
					? api.PUT('/api/security/sso/providers/{id}', { params: { path: { id } }, body: f })
					: api.POST('/api/security/sso/providers', { body: f })
			);
			open = false;
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
			<Sheet.Title>{provider ? `Edit ${provider.name}` : 'Add a sign-in provider'}</Sheet.Title>
		</Sheet.Header>
		<div class="space-y-3 overflow-y-auto px-4 pb-4 text-md">
			<div class="space-y-1.5">
				<Label for="pn">Name</Label><Input id="pn" bind:value={f.name} maxlength={32} />
			</div>
			<div class="space-y-1.5">
				<Label for="pi">Issuer</Label><Input
					id="pi"
					class="font-mono"
					placeholder="https://sso.example.com"
					bind:value={f.issuer}
				/>
			</div>
			<div class="grid gap-3 sm:grid-cols-2">
				<div class="space-y-1.5">
					<Label for="pc">Client ID</Label><Input
						id="pc"
						class="font-mono"
						bind:value={f.client_id}
					/>
				</div>
				<div class="space-y-1.5">
					<Label for="ps">Client secret</Label><Input
						id="ps"
						type="password"
						placeholder={provider ? 'Stored' : ''}
						bind:value={f.client_secret}
					/>
				</div>
			</div>
			{#if callback}<p class="text-sm text-muted-foreground">
					Callback address at the provider: <Mono class="break-all">{callback}</Mono>
				</p>{/if}
			<label class="flex items-center gap-2"
				><Switch bind:checked={f.is_default} />Default provider</label
			>
			<MoreOptions {changed}>
				<div class="space-y-1.5">
					<Label for="pp">Scopes</Label><Input id="pp" class="font-mono" bind:value={f.scopes} />
				</div>
				<div class="grid gap-3 sm:grid-cols-3">
					<div class="space-y-1.5">
						<Label for="pu">Username claim</Label><Input
							id="pu"
							class="font-mono"
							bind:value={f.username_claim}
						/>
					</div>
					<div class="space-y-1.5">
						<Label for="pe">Email claim</Label><Input
							id="pe"
							class="font-mono"
							bind:value={f.email_claim}
						/>
					</div>
					<div class="space-y-1.5">
						<Label for="pg">Groups claim</Label><Input
							id="pg"
							class="font-mono"
							bind:value={f.groups_claim}
						/>
					</div>
				</div>
			</MoreOptions>
			{#if error}<p role="alert" class="text-bad-text">{error}</p>{/if}
		</div>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			<Button variant="outline" onclick={() => (open = false)}>Cancel</Button>
			<Button disabled={busy || !f.name.trim() || !f.issuer.trim()} onclick={save}
				>{provider ? 'Save changes' : 'Add provider'}</Button
			>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>
