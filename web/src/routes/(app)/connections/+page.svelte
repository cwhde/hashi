<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import TargetSources from '$lib/components/hashi/TargetSources.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import * as Select from '$lib/components/ui/select';
	import * as Sheet from '$lib/components/ui/sheet';
	import { Pencil, Plus, RefreshCw, Trash2 } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { withReauth } from '$lib/auth/reauth';

	// 03 §8: AdGuard connections. Other connection kinds keep their own pages for now.
	type Conn = Schemas['AdGuardConnection'];
	let conns = $state<Conn[]>([]);
	let sets = $state<{ id: string; name: string }[]>([]);
	let error = $state('');
	let busy = $state(false);

	const modes: Record<string, string> = {
		read_only: 'Read-only: only looks, changes nothing',
		active: 'Active: Hashi manages its own rewrites',
		disabled: 'Disabled: not contacted'
	};
	const modeTag = { read_only: 'Read-only', active: 'Active', disabled: 'Disabled' };

	async function load() {
		const [c, s] = await Promise.all([
			api.GET('/api/adguard/connections'),
			api.GET('/api/ip-sets', { params: { query: {} } })
		]);
		if (!c.data || !s.data) throw c.error ?? s.error;
		conns = c.data.connections;
		sets = s.data.sets.map((x) => ({ id: x.id, name: x.name }));
	}
	onMount(() => {
		load().catch((e) => (error = problemText(e)));
	});

	async function test(id: string) {
		error = '';
		busy = true;
		try {
			const r = await api.POST('/api/adguard/connections/{id}/test', { params: { path: { id } } });
			if (r.error) throw r.error;
			await load();
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = false;
		}
	}
	async function remove(id: string) {
		error = '';
		busy = true;
		try {
			await withReauth(() =>
				api.DELETE('/api/adguard/connections/{id}', { params: { path: { id } } })
			);
			await load();
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = false;
		}
	}

	// ---- add and edit ----
	const blank = () => ({
		name: '',
		target_set_id: '',
		port: '443',
		scheme: 'https',
		path_prefix: '',
		tls_verify: true,
		server_name: '',
		username: '',
		password: '',
		mode: 'read_only' as Conn['mode'],
		save_inactive: false
	});
	let open = $state(false);
	let editing = $state<string | null>(null);
	let f = $state(blank());
	let formError = $state('');
	function openAdd() {
		f = blank();
		editing = null;
		formError = '';
		open = true;
	}
	function openEdit(c: Conn) {
		f = { ...blank(), ...c, port: String(c.port), password: '' };
		editing = c.id;
		formError = '';
		open = true;
	}
	async function save() {
		formError = '';
		busy = true;
		try {
			const body = {
				name: f.name,
				target_set_id: f.target_set_id,
				port: Number(f.port),
				scheme: f.scheme,
				path_prefix: f.path_prefix || undefined,
				tls_verify: f.tls_verify,
				server_name: f.server_name || undefined,
				username: f.username || undefined,
				password: f.password || undefined,
				mode: f.mode,
				save_inactive: f.save_inactive
			};
			const id = editing;
			const row = await withReauth(() =>
				id
					? api.PUT('/api/adguard/connections/{id}', { params: { path: { id } }, body })
					: api.POST('/api/adguard/connections', { body })
			);
			f.password = '';
			open = false;
			await load();
			if (row?.id) await test(row.id);
		} catch (e) {
			formError = problemText(e);
		} finally {
			busy = false;
		}
	}
</script>

<PageHeader
	title="Connections"
	description="The outside systems Hashi talks to. Credentials are kept in the vault and are never shown again after saving."
>
	{#snippet actions()}<Button onclick={openAdd}><Plus />Add AdGuard</Button>{/snippet}
</PageHeader>

{#if error}<p role="alert" class="mb-3 text-[13px] text-bad-text">{error}</p>{/if}
<div class="rounded-2xl border bg-card px-4 py-3">
	<h2 class="mb-1 text-[12.5px] font-medium text-muted-foreground">AdGuard Home</h2>
	{#if conns.length === 0}
		<p class="py-2 text-[13px] text-muted-foreground">
			No AdGuard instance yet. A new one starts read-only, so adding it changes nothing.
		</p>
	{/if}
	<ul>
		{#each conns as c (c.id)}
			<li class="flex flex-wrap items-center gap-3 py-2 text-[13px]">
				<span class="min-w-40 font-medium">{c.name}</span>
				<span class="min-w-0 flex-1 truncate text-muted-foreground">
					{#if c.last_error}<span class="text-bad-text">{c.last_error}</span
						>{:else if c.last_version}{c.last_version}{:else}Not checked yet{/if}
				</span>
				<Tag tone={c.mode === 'active' ? 'ok' : undefined}>{modeTag[c.mode]}</Tag>
				<Button variant="outline" size="sm" href="/dns?tab=adguard">Show rewrites</Button>
				<Button
					variant="ghost"
					size="icon-sm"
					aria-label="Test {c.name}"
					onclick={() => test(c.id)}
					disabled={busy}><RefreshCw /></Button
				>
				<Button
					variant="ghost"
					size="icon-sm"
					aria-label="Edit {c.name}"
					onclick={() => openEdit(c)}
					disabled={busy}><Pencil /></Button
				>
				<Button
					variant="ghost"
					size="icon-sm"
					aria-label="Remove {c.name}"
					onclick={() => remove(c.id)}
					disabled={busy}><Trash2 /></Button
				>
			</li>
		{/each}
	</ul>
</div>

<Sheet.Root bind:open>
	<Sheet.Content class="w-full sm:max-w-md">
		<Sheet.Header>
			<Sheet.Title>{editing ? 'Edit AdGuard' : 'Add AdGuard'}</Sheet.Title>
			<Sheet.Description
				>{editing
					? 'Leave the password empty to keep the stored one.'
					: 'The password is stored encrypted and never shown again.'}</Sheet.Description
			>
		</Sheet.Header>
		<div class="space-y-4 overflow-y-auto px-4">
			<div class="space-y-1.5">
				<Label for="ag-name">Name</Label><Input id="ag-name" bind:value={f.name} />
			</div>
			<div class="space-y-1.5">
				<Label for="ag-set">Address set</Label>
				<Select.Root type="single" bind:value={f.target_set_id}>
					<Select.Trigger id="ag-set" class="w-full"
						>{sets.find((s) => s.id === f.target_set_id)?.name ?? 'Choose a set'}</Select.Trigger
					>
					<Select.Content>
						{#each sets as s (s.id)}<Select.Item value={s.id}>{s.name}</Select.Item>{/each}
					</Select.Content>
				</Select.Root>
				<TargetSources setId={f.target_set_id} />
				<p class="text-[12px] text-muted-foreground">
					An IP set holding the instance's address or DNS name. With a Pulse agent in it, Hashi
					needs HTTPS, certificate checks and the expected hostname, so the password only goes to
					the real instance.
				</p>
			</div>
			<div class="grid grid-cols-[1fr_6rem] gap-3">
				<div class="space-y-1.5">
					<Label for="ag-scheme">Scheme</Label>
					<Select.Root type="single" bind:value={f.scheme}>
						<Select.Trigger id="ag-scheme" class="w-full">{f.scheme.toUpperCase()}</Select.Trigger>
						<Select.Content>
							<Select.Item value="https">HTTPS</Select.Item>
							<Select.Item value="http">HTTP</Select.Item>
						</Select.Content>
					</Select.Root>
				</div>
				<div class="space-y-1.5">
					<Label for="ag-port">Port</Label><Input
						id="ag-port"
						bind:value={f.port}
						inputmode="numeric"
					/>
				</div>
			</div>
			<div class="space-y-1.5">
				<Label for="ag-host">Expected hostname</Label><Input
					id="ag-host"
					bind:value={f.server_name}
					placeholder="ns.example.com"
				/>
				<p class="text-[12px] text-muted-foreground">The name on its certificate.</p>
			</div>
			<label class="flex items-center justify-between gap-3 text-[13px]">
				<span>Check the certificate</span><Switch
					bind:checked={f.tls_verify}
					aria-label="Check the certificate"
				/>
			</label>
			<div class="space-y-1.5">
				<Label for="ag-user">Username</Label><Input
					id="ag-user"
					autocomplete="off"
					bind:value={f.username}
				/>
			</div>
			<div class="space-y-1.5">
				<Label for="ag-pass">Password</Label><Input
					id="ag-pass"
					type="password"
					autocomplete="off"
					bind:value={f.password}
				/>
			</div>
			<div class="space-y-1.5">
				<Label for="ag-mode">Mode</Label>
				<Select.Root type="single" bind:value={f.mode}>
					<Select.Trigger id="ag-mode" class="w-full">{modes[f.mode]}</Select.Trigger>
					<Select.Content>
						{#each Object.entries(modes) as [v, t] (v)}<Select.Item value={v}>{t}</Select.Item
							>{/each}
					</Select.Content>
				</Select.Root>
			</div>
			<label class="flex items-center justify-between gap-3 text-[13px]">
				<span>Save even if it does not answer yet</span><Switch
					bind:checked={f.save_inactive}
					aria-label="Save even if it does not answer yet"
				/>
			</label>
			{#if formError}<p role="alert" class="text-[13px] text-bad-text">{formError}</p>{/if}
		</div>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			<Button variant="outline" onclick={() => (open = false)}>Cancel</Button>
			<Button onclick={save} disabled={busy || !f.name.trim() || !f.target_set_id}
				>{editing ? 'Save changes' : 'Add AdGuard'}</Button
			>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>
