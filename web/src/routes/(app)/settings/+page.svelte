<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { cn } from '$lib/utils';
	import { Fingerprint, KeyRound, Trash2, Lock, Pencil } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { withReauth } from '$lib/auth/reauth';
	import { createPasskey, ceremonyText } from '$lib/auth/webauthn';
	import { dateTime, time } from '$lib/format';
	import { theme, themes } from '$lib/theme.svelte';
	import Notifications from '$lib/components/hashi/status/Notifications.svelte';
	import AgentDns from '$lib/components/hashi/pulse/AgentDns.svelte';
	import PulseSettings from '$lib/components/hashi/pulse/PulseSettings.svelte';

	const cats = [
		{ id: 'general', label: 'General' },
		{ id: 'account', label: 'Account' },
		{ id: 'appearance', label: 'Appearance' },
		{ id: 'security', label: 'Security' },
		{ id: 'notifications', label: 'Notifications' },
		{ id: 'pulse', label: 'Pulse' },
		{ id: 'agent-dns', label: 'Internal agent DNS' }
	];
	const cat = $derived(page.url.searchParams.get('category') ?? 'general');
	function show(id: string) {
		goto(`?category=${id}`, { replaceState: true, keepFocus: true, noScroll: true });
	}

	// Nothing is saved that failed to load (09 §2): forms render only after a successful load.
	let all = $state<Schemas['AllSettings'] | null>(null);
	let loadError = $state('');
	let general = $state<Schemas['GeneralSettings'] | null>(null);
	let proxies = $state('');
	let security = $state<Schemas['SecuritySettings'] | null>(null);
	let cidrs = $state('');
	let appearance = $state<Schemas['AppearanceSettings'] | null>(null);

	async function loadSettings() {
		const res = await api.GET('/api/settings');
		if (res.error || !res.data) {
			loadError = problemText(res.error);
			return;
		}
		all = res.data;
		general = { ...res.data.general };
		proxies = res.data.general.trusted_proxies.join('\n');
		security = { ...res.data.security };
		cidrs = res.data.security.bootstrap_allowed_cidrs.join('\n');
		appearance = { ...res.data.appearance };
	}
	const list = (s: string) =>
		s
			.split(/[\s,]+/)
			.map((x) => x.trim())
			.filter(Boolean);

	// One status line per form: what happened, or why nothing changed.
	let status = $state<Record<string, { ok: boolean; msg: string }>>({});
	let busy = $state('');
	async function save(key: string, fn: () => Promise<unknown>, done: string) {
		busy = key;
		status[key] = { ok: true, msg: '' };
		try {
			await fn();
			status[key] = { ok: true, msg: done };
		} catch (e) {
			status[key] = { ok: false, msg: e instanceof Error ? ceremonyText(e) : problemText(e) };
		} finally {
			busy = '';
		}
	}
	const saveGeneral = () =>
		save(
			'general',
			async () => {
				const body = { ...general!, trusted_proxies: list(proxies) };
				general = await withReauth(() => api.PUT('/api/settings/general', { body }));
			},
			'Saved.'
		);
	const saveSecurity = () =>
		save(
			'security',
			async () => {
				const body = { ...security!, bootstrap_allowed_cidrs: list(cidrs) };
				security = await withReauth(() => api.PUT('/api/settings/security', { body }));
			},
			'Saved. New limits apply to every session from its next request.'
		);
	const saveAppearance = () =>
		save(
			'appearance',
			async () => {
				appearance = await withReauth(() =>
					api.PUT('/api/settings/appearance', { body: { ...appearance!, theme: theme.current } })
				);
			},
			'Saved.'
		);

	// Account
	let passkeys = $state<Schemas['Passkey'][] | null>(null);
	let sessions = $state<Schemas['SessionInfo'][] | null>(null);
	let vault = $state<Schemas['VaultState'] | null>(null);
	let accountError = $state('');
	async function loadAccount() {
		const [p, s, v] = await Promise.all([
			api.GET('/api/auth/passkeys'),
			api.GET('/api/auth/sessions'),
			api.GET('/api/vault')
		]);
		if (p.error || s.error || v.error) {
			accountError = problemText(p.error ?? s.error ?? v.error);
			return;
		}
		passkeys = p.data!.passkeys;
		sessions = s.data!.sessions;
		vault = v.data!;
	}
	let newName = $state('');
	const addPasskey = () =>
		save(
			'passkey',
			async () => {
				const opts = await withReauth(() => api.POST('/api/auth/passkeys/register/options'));
				const credential = await createPasskey(opts.options);
				await withReauth(() =>
					api.POST('/api/auth/passkeys/register/verify', {
						body: { ceremony_id: opts.ceremony_id, name: newName.trim() || 'Passkey', credential }
					})
				);
				newName = '';
				await loadAccount();
			},
			'Passkey added.'
		);
	let renaming = $state<string | null>(null);
	let renameTo = $state('');
	const rename = (id: string) =>
		save(
			'passkey',
			async () => {
				const res = await api.PATCH('/api/auth/passkeys/{id}', {
					params: { path: { id } },
					body: { name: renameTo }
				});
				if (res.error) throw res.error;
				renaming = null;
				await loadAccount();
			},
			'Renamed.'
		);
	let deleting = $state<Schemas['Passkey'] | null>(null);
	const remove = (pk: Schemas['Passkey']) =>
		save(
			'passkey',
			async () => {
				await withReauth(() =>
					api.DELETE('/api/auth/passkeys/{id}', { params: { path: { id: pk.id } } })
				);
				deleting = null;
				await loadAccount();
			},
			`Deleted “${pk.name}”; its sessions are signed out.`
		);
	const revoke = (id: string) =>
		save(
			'sessions',
			async () => {
				await withReauth(() => api.DELETE('/api/auth/sessions/{id}', { params: { path: { id } } }));
				await loadAccount();
			},
			'Session signed out.'
		);
	const revokeOthers = () =>
		save(
			'sessions',
			async () => {
				const r = await withReauth(() => api.POST('/api/auth/sessions/revoke-others'));
				await loadAccount();
				status.sessions = {
					ok: true,
					msg: `${r.revoked} other session${r.revoked === 1 ? '' : 's'} signed out.`
				};
			},
			''
		);

	onMount(() => {
		loadSettings();
		loadAccount();
	});
</script>

<svelte:head><title>Settings · Hashi</title></svelte:head>
{#snippet note(key: string)}
	{#if status[key]?.msg}
		<p
			role={status[key].ok ? 'status' : 'alert'}
			class={cn('text-[13px]', status[key].ok ? 'text-ok-text' : 'text-bad-text')}
		>
			{status[key].msg}
		</p>
	{/if}
{/snippet}
{#snippet loading()}
	<Skeleton class="h-40 rounded-2xl" />
{/snippet}

<PageHeader
	title="Settings"
	description="Every setting here changes behaviour somewhere; nothing is saved that failed to load."
/>
<div class="grid gap-5 lg:grid-cols-[14rem_minmax(0,1fr)]">
	<nav aria-label="Settings categories" class="flex gap-1 overflow-x-auto lg:flex-col">
		{#each cats as c (c.id)}
			<button
				onclick={() => show(c.id)}
				aria-current={cat === c.id ? 'page' : undefined}
				class={cn(
					'h-9 shrink-0 rounded-[10px] px-3 text-left text-[13.5px]',
					cat === c.id ? 'bg-surface-2 font-medium' : 'text-muted-foreground hover:text-foreground'
				)}>{c.label}</button
			>
		{/each}
		<p class="hidden px-3 pt-3 text-[12px] text-muted-foreground lg:block">
			More categories arrive with the features they configure.
		</p>
	</nav>

	<div class="min-w-0 space-y-4">
		{#if cat === 'account'}
			{#if accountError}
				<p role="alert" class="text-[13px] text-bad-text">{accountError}</p>
			{:else if !passkeys || !sessions || !vault}
				{@render loading()}{@render loading()}
			{:else}
				<Panel title="Passkeys" description="Sign-in and confirmation of risky changes" flush>
					{#snippet actions()}
						<Input
							bind:value={newName}
							placeholder="Name"
							aria-label="New passkey name"
							maxlength={64}
							class="h-8 w-36"
						/>
						<Button size="sm" variant="outline" onclick={addPasskey} disabled={busy === 'passkey'}
							><Fingerprint />Add passkey</Button
						>
					{/snippet}
					<ul class="divide-y text-[13px]">
						{#each passkeys as pk (pk.id)}
							<li class="flex items-center gap-3 px-4 py-3">
								<KeyRound class="size-4 text-muted-foreground" />
								{#if renaming === pk.id}
									<form
										class="flex flex-1 gap-2"
										onsubmit={(e) => (e.preventDefault(), rename(pk.id))}
									>
										<Input
											bind:value={renameTo}
											maxlength={64}
											class="h-8 max-w-56"
											aria-label="Passkey name"
										/>
										<Button size="sm" type="submit">Save</Button>
										<Button size="sm" variant="ghost" onclick={() => (renaming = null)}
											>Cancel</Button
										>
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
										onclick={() => (deleting = pk)}><Trash2 /></Button
									>
								{/if}
							</li>
						{/each}
					</ul>
					<div class="px-4 pb-3">{@render note('passkey')}</div>
				</Panel>

				<Panel
					title="Active sessions"
					description="Each session is tied to the IP it started from and ends after 8 h at most"
					flush
				>
					{#snippet actions()}<Button
							size="sm"
							variant="outline"
							onclick={revokeOthers}
							disabled={busy === 'sessions' || (sessions?.length ?? 0) < 2}
							>Sign out other sessions</Button
						>{/snippet}
					<ul class="divide-y text-[13px]">
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
					<div class="px-4 pb-3">{@render note('sessions')}</div>
				</Panel>

				<Panel title="Vault">
					<div class="flex items-start gap-3 text-[13px]">
						<Lock class="mt-0.5 size-4 text-muted-foreground" />
						<div class="flex-1 space-y-1">
							<p>
								Service mode: secrets unlock automatically with the deployment's master key, so
								syncs keep running after a restart. Strict mode keeps them locked until you unlock
								with a passkey or the recovery key.
							</p>
							<p class="text-muted-foreground">
								{vault.secrets} stored secret{vault.secrets === 1 ? '' : 's'} · master key version {vault.key_version}
								· recovery key
								{vault.recovery_key_confirmed ? 'confirmed' : 'not confirmed'}
							</p>
						</div>
						<Button
							size="sm"
							variant="outline"
							disabled
							title="Strict mode arrives in a later slice">Change mode…</Button
						>
					</div>
				</Panel>
			{/if}
		{:else if loadError}
			<p
				role="alert"
				class="rounded-xl border border-bad/30 bg-bad/10 p-3 text-[13px] text-bad-text"
			>
				{loadError} The form stays hidden so nothing is saved from an incomplete load.
			</p>
		{:else if !all}
			{@render loading()}
		{:else if cat === 'notifications'}
			<Notifications />
		{:else if cat === 'pulse'}
			<PulseSettings />
		{:else if cat === 'agent-dns'}
			<AgentDns />
		{:else if cat === 'general' && general}
			<Panel title="Domains and addresses">
				<div class="grid gap-4 md:grid-cols-2">
					<div class="space-y-1.5">
						<Label for="rd">Root domain</Label><Input
							id="rd"
							value={general.root_domain}
							disabled
						/>
						<p class="text-[12px] text-muted-foreground">
							Fixed after setup: your passkeys are bound to it.
						</p>
					</div>
					<div class="space-y-1.5">
						<Label for="ad">Admin domain</Label><Input id="ad" bind:value={general.admin_domain} />
					</div>
					<div class="space-y-1.5">
						<Label for="iu">Internal Hashi URL</Label><Input
							id="iu"
							bind:value={general.internal_url}
							class="font-mono"
						/>
					</div>
					<div class="space-y-1.5">
						<Label for="si">Reconcile every (minutes)</Label><Input
							id="si"
							type="number"
							min="1"
							max="1440"
							value={general.sync_interval_seconds / 60}
							oninput={(e) =>
								(general!.sync_interval_seconds = Math.round(Number(e.currentTarget.value) * 60))}
						/>
					</div>
				</div>
			</Panel>
			<Panel
				title="Trusted proxies"
				description="Forwarded client IPs are believed only from these. Changing this asks for your passkey."
			>
				<Label for="tp" class="flex items-center gap-1.5"
					>Addresses or CIDRs, one per line<Fingerprint
						class="size-3.5 text-muted-foreground"
						aria-label="Changing this asks for your passkey"
					/></Label
				>
				<Textarea
					id="tp"
					bind:value={proxies}
					rows={3}
					class="mt-1.5 font-mono"
					placeholder="192.0.2.11/32"
				/>
			</Panel>
			<div class="flex items-center justify-end gap-3">
				{@render note('general')}<Button
					size="sm"
					onclick={saveGeneral}
					disabled={busy === 'general'}>Save</Button
				>
			</div>
		{:else if cat === 'security' && security}
			<Panel title="Admin sessions">
				<div class="grid gap-4 md:grid-cols-2">
					<div class="space-y-1.5">
						<Label for="it">Sign out after inactivity (minutes)</Label>
						<Input
							id="it"
							type="number"
							min="5"
							max="240"
							bind:value={security.admin_idle_timeout_minutes}
						/>
						<p class="text-[12px] text-muted-foreground">
							5 to 240. Sessions always end 8 hours after sign-in.
						</p>
					</div>
				</div>
			</Panel>
			<Panel
				title="Bootstrap networks"
				description="Where the one-time bootstrap login and recovery links work from"
			>
				<Textarea bind:value={cidrs} rows={4} class="font-mono" aria-label="Bootstrap networks" />
			</Panel>
			<div class="flex items-center justify-end gap-3">
				{@render note('security')}<Button
					size="sm"
					onclick={saveSecurity}
					disabled={busy === 'security'}>Save</Button
				>
			</div>
		{:else if cat === 'appearance' && appearance}
			<Panel title="Theme">
				<div class="grid grid-cols-2 gap-3 md:grid-cols-4">
					{#each themes as t (t.id)}
						<button
							onclick={() => void theme.save(t.id)}
							aria-pressed={theme.current === t.id}
							class={cn(
								'rounded-xl border p-1.5 text-left',
								theme.current === t.id && 'ring-2 ring-ring'
							)}
						>
							<div
								data-theme={t.id}
								data-mode={t.mode}
								class="flex h-20 gap-1.5 rounded-lg border bg-background p-1.5"
							>
								<div class="w-5 rounded-md bg-sidebar"></div>
								<div class="flex flex-1 flex-col justify-between rounded-md border bg-card p-1.5">
									<div class="h-1.5 w-3/4 rounded-full bg-muted-foreground/40"></div>
									<div class="h-2.5 w-8 self-end rounded bg-primary"></div>
								</div>
							</div>
							<div class="px-1 pt-1.5 text-[13px] font-medium">{t.name}</div>
						</button>
					{/each}
				</div>
			</Panel>
			<Panel title="Public pages">
				<div class="grid gap-3 md:grid-cols-2">
					<label
						class="flex items-center justify-between gap-3 rounded-xl border px-3 py-2.5 text-[13px]"
						>Public app dashboard<Switch
							bind:checked={appearance.public_dashboard_enabled}
						/></label
					>
					<label
						class="flex items-center justify-between gap-3 rounded-xl border px-3 py-2.5 text-[13px]"
						>Public status page<Switch bind:checked={appearance.public_status_enabled} /></label
					>
				</div>
			</Panel>
			<div class="flex items-center justify-end gap-3">
				{@render note('appearance')}<Button
					size="sm"
					onclick={saveAppearance}
					disabled={busy === 'appearance'}>Save</Button
				>
			</div>
		{/if}
	</div>
</div>

<AlertDialog.Root open={!!deleting} onOpenChange={(o) => !o && (deleting = null)}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>Delete passkey “{deleting?.name}”?</AlertDialog.Title>
			<AlertDialog.Description
				>Sessions signed in with it end immediately. Hashi asks for a passkey to confirm.</AlertDialog.Description
			>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>Keep it</AlertDialog.Cancel>
			<AlertDialog.Action
				class="border border-bad/30 bg-bad/15 text-bad-text hover:bg-bad/25"
				onclick={() => deleting && remove(deleting)}>Delete passkey</AlertDialog.Action
			>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
