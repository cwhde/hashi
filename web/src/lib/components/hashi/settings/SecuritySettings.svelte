<script lang="ts">
	// Settings → Security (10c §1.2, bug 36): admin sign-in. Passkeys, sessions, the vault, and the networks Hashi
	// trusts. Proxies and bootstrap networks take private ranges only (bugs 48, 49).
	import { onMount } from 'svelte';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import Tag from '#lib/components/hashi/Tag.svelte';
	import Mono from '#lib/components/hashi/Mono.svelte';
	import VaultPanel from '#lib/components/hashi/VaultPanel.svelte';
	import AddPasskeyDialog from '#lib/components/hashi/AddPasskeyDialog.svelte';
	import ConfirmDelete from '#lib/components/hashi/ConfirmDelete.svelte';
	import SaveBar from './SaveBar.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import { Textarea } from '#lib/components/ui/textarea/index.ts';
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	import { Fingerprint, KeyRound, Trash2, Pencil } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { withReauth } from '#lib/auth/reauth.ts';
	import { dateTime, time } from '#lib/format.ts';
	import { vaultState } from '#lib/vault.svelte.ts';
	import { Draft, guardUnsaved } from '#lib/unsaved.svelte.ts';
	import { Saver, lines } from '#lib/settings/saver.svelte.ts';

	let passkeys = $state<Schemas['Passkey'][] | null>(null);
	let sessions = $state<Schemas['SessionInfo'][] | null>(null);
	let vault = $state<Schemas['VaultState'] | null>(null);
	let error = $state('');
	async function loadAccess() {
		const [p, s, v] = await Promise.all([
			api.GET('/api/auth/passkeys'),
			api.GET('/api/auth/sessions'),
			api.GET('/api/vault')
		]);
		if (p.error || s.error || v.error) {
			error = problemText(p.error ?? s.error ?? v.error);
			return;
		}
		passkeys = p.data!.passkeys;
		sessions = s.data!.sessions;
		vault = v.data!;
		vaultState.set(v.data!);
	}

	// ---- passkeys ----
	const keys = new Saver();
	let adding = $state(false);
	let renaming = $state<string | null>(null);
	let renameTo = $state('');
	let deleting = $state<Schemas['Passkey'] | null>(null);
	let confirmOpen = $state(false);
	const rename = (id: string) =>
		keys.run(async () => {
			const r = await api.PATCH('/api/auth/passkeys/{id}', {
				params: { path: { id } },
				body: { name: renameTo }
			});
			if (r.error) throw r.error;
			renaming = null;
			await loadAccess();
		}, 'Renamed.');
	const removeKey = (pk: Schemas['Passkey']) =>
		keys.run(async () => {
			await withReauth(() =>
				api.DELETE('/api/auth/passkeys/{id}', { params: { path: { id: pk.id } } })
			);
			await loadAccess();
		}, `Deleted “${pk.name}”; its sessions are signed out.`);

	// ---- sessions ----
	const sess = new Saver();
	const revoke = (id: string) =>
		sess.run(async () => {
			await withReauth(() => api.DELETE('/api/auth/sessions/{id}', { params: { path: { id } } }));
			await loadAccess();
		}, 'Signed out.');
	const revokeOthers = () =>
		sess.run(async () => {
			const r = await withReauth(() => api.POST('/api/auth/sessions/revoke-others'));
			await loadAccess();
			sess.note = {
				ok: true,
				msg: `${r.revoked} other session${r.revoked === 1 ? '' : 's'} signed out.`
			};
		}, '');

	// ---- sign-in limits and networks (security record) ----
	let sec = $state<Schemas['SecuritySettings'] | null>(null);
	let cidrs = $state('');
	const secDraft = new Draft(() => [sec, cidrs]);
	const secSaver = new Saver();
	const saveSec = () =>
		secSaver.run(
			async () => {
				const body = { ...sec!, bootstrap_allowed_cidrs: lines(cidrs) };
				sec = await withReauth(() => api.PUT('/api/settings/security', { body }));
				cidrs = sec.bootstrap_allowed_cidrs.join('\n');
			},
			'Saved. Sessions follow it from their next request.',
			secDraft
		);

	// ---- trusted proxies (general record, the rest of it is under General) ----
	let proxies = $state('');
	const proxyDraft = new Draft(() => proxies);
	const proxySaver = new Saver();
	const saveProxies = () =>
		proxySaver.run(
			async () => {
				const cur = await api.GET('/api/settings/general');
				if (!cur.data) throw cur.error;
				const body = { ...cur.data, trusted_proxies: lines(proxies) };
				const g = await withReauth(() => api.PUT('/api/settings/general', { body }));
				proxies = g.trusted_proxies.join('\n');
			},
			'Saved.',
			proxyDraft
		);
	guardUnsaved(() => secDraft.dirty || proxyDraft.dirty);

	let settingsError = $state('');
	onMount(async () => {
		void loadAccess();
		const [s, g] = await Promise.all([
			api.GET('/api/settings/security'),
			api.GET('/api/settings/general')
		]);
		if (!s.data || !g.data) {
			settingsError = problemText(s.error ?? g.error);
			return;
		}
		sec = s.data;
		cidrs = s.data.bootstrap_allowed_cidrs.join('\n');
		proxies = g.data.trusted_proxies.join('\n');
		secDraft.mark();
		proxyDraft.mark();
	});
</script>

{#if error}<p role="alert" class="text-md text-bad-text">{error}</p>
{:else if !passkeys || !sessions || !vault}<Skeleton class="h-40 rounded-2xl" />
{:else}
	<Panel title="Passkeys" flush>
		{#snippet actions()}
			<Button size="sm" variant="outline" onclick={() => (adding = true)}
				><Fingerprint />Add passkey</Button
			>
		{/snippet}
		<ul class="divide-y text-md">
			{#each passkeys as pk (pk.id)}
				<li class="flex items-center gap-3 px-4 py-3">
					<KeyRound class="size-4 text-muted-foreground" />
					{#if renaming === pk.id}
						<form class="flex flex-1 gap-2" onsubmit={(e) => (e.preventDefault(), rename(pk.id))}>
							<Input
								bind:value={renameTo}
								maxlength={64}
								class="h-8 max-w-56"
								aria-label="Passkey name"
							/>
							<Button size="sm" type="submit">Save</Button>
							<Button size="sm" variant="ghost" onclick={() => (renaming = null)}>Cancel</Button>
						</form>
					{:else}
						<span class="flex-1"
							><span class="font-medium">{pk.name}</span>
							<span class="text-muted-foreground"
								>· added {dateTime(pk.created_at)} · {pk.last_used_at
									? `last used ${dateTime(pk.last_used_at)}`
									: 'not used yet'}</span
							></span
						>
						<Button
							variant="ghost"
							size="icon-sm"
							aria-label="Rename passkey {pk.name}"
							onclick={() => ((renaming = pk.id), (renameTo = pk.name))}><Pencil /></Button
						>
						<Button
							variant="ghost"
							size="icon-sm"
							class="text-bad-text"
							aria-label="Delete passkey {pk.name}"
							disabled={passkeys.length === 1}
							title={passkeys.length === 1 ? 'The last passkey cannot be deleted' : undefined}
							onclick={() => ((deleting = pk), (confirmOpen = true))}><Trash2 /></Button
						>
					{/if}
				</li>
			{/each}
		</ul>
		{#if keys.note}<p
				role={keys.note.ok ? 'status' : 'alert'}
				class="px-4 pb-3 text-md {keys.note.ok ? 'text-ok-text' : 'text-bad-text'}"
			>
				{keys.note.msg}
			</p>{/if}
	</Panel>

	<Panel title="Active sessions" flush>
		{#snippet actions()}<Button
				size="sm"
				variant="outline"
				onclick={revokeOthers}
				disabled={sess.busy || (sessions?.length ?? 0) < 2}>Sign out other sessions</Button
			>{/snippet}
		<ul class="divide-y text-md">
			{#each sessions as s (s.id)}
				<li class="flex items-center gap-3 px-4 py-3">
					<span class="flex-1"
						><Mono>{s.bound_ip}</Mono> · started {dateTime(s.created_at)} · active {dateTime(
							s.last_seen_at
						)} · ends {time(s.absolute_expires_at)}</span
					>
					{#if s.current}<Tag tone="accent">this session</Tag>{:else}<Button
							size="xs"
							variant="ghost"
							onclick={() => revoke(s.id)}>Sign out</Button
						>{/if}
				</li>
			{/each}
		</ul>
		{#if sess.note}<p
				role={sess.note.ok ? 'status' : 'alert'}
				class="px-4 pb-3 text-md {sess.note.ok ? 'text-ok-text' : 'text-bad-text'}"
			>
				{sess.note.msg}
			</p>{/if}
	</Panel>

	<VaultPanel vault={vaultState.current ?? vault} {passkeys} onchanged={loadAccess} />
	<AddPasskeyDialog bind:open={adding} for="signin" {passkeys} onchanged={loadAccess} />
	<ConfirmDelete
		bind:open={confirmOpen}
		title="Delete passkey “{deleting?.name}”?"
		body="Sessions signed in with it end at once. Hashi asks for a passkey to confirm."
		verb="Delete passkey"
		onconfirm={() => deleting && void removeKey(deleting)}
	/>
{/if}

{#if settingsError}<p role="alert" class="text-md text-bad-text">{settingsError}</p>
{:else if sec}
	<Panel title="Sign-in limits">
		<div class="grid gap-4 md:grid-cols-2">
			<div class="space-y-1.5">
				<Label for="it">Sign out after inactivity (minutes)</Label>
				<Input
					id="it"
					type="number"
					min="5"
					max="240"
					bind:value={sec.admin_idle_timeout_minutes}
				/>
				<p class="text-xs text-muted-foreground">Sessions always end 8 hours after sign-in.</p>
			</div>
			<div class="space-y-1.5">
				<Label for="bn">Recovery links work from</Label>
				<Textarea
					id="bn"
					bind:value={cidrs}
					rows={3}
					class="font-mono"
					placeholder="192.168.0.0/16"
				/>
			</div>
		</div>
		<div class="mt-3"><SaveBar saver={secSaver} dirty={secDraft.dirty} onsave={saveSec} /></div>
	</Panel>
	<Panel title="Trusted proxies">
		<Label for="tp" class="flex items-center gap-1.5"
			>Believe forwarded addresses from<Fingerprint
				class="size-3.5 text-muted-foreground"
				aria-label="Changing this asks for your passkey"
			/></Label
		>
		<Textarea
			id="tp"
			bind:value={proxies}
			rows={3}
			class="mt-1.5 font-mono"
			placeholder="10.0.0.11/32"
		/>
		<div class="mt-3">
			<SaveBar saver={proxySaver} dirty={proxyDraft.dirty} onsave={saveProxies} />
		</div>
	</Panel>
{:else}<Skeleton class="h-40 rounded-2xl" />
{/if}
