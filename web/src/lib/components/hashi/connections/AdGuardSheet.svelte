<script lang="ts">
	// Add or edit one AdGuard Home connection (03 §8). How it is reached: a Pulse agent (followed) or an IP set.
	import { onMount } from 'svelte';
	import TargetSources from '#lib/components/hashi/TargetSources.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import { Switch } from '#lib/components/ui/switch/index.ts';
	import * as Select from '#lib/components/ui/select/index.ts';
	import * as Sheet from '#lib/components/ui/sheet/index.ts';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { withReauth } from '#lib/auth/reauth.ts';
	import { pulseAgents } from '#lib/pulseAgents.svelte.ts';
	import { targetNames } from '#lib/targetNames.svelte.ts';

	type Conn = Schemas['AdGuardConnection'];
	let {
		open = $bindable(false),
		conn = null,
		onsaved
	}: { open?: boolean; conn?: Conn | null; onsaved: (id: string) => void } = $props();
	onMount(() => void targetNames.load());

	const modes: Record<string, string> = {
		read_only: 'Read-only: only looks, changes nothing',
		active: 'Active: Hashi manages its own rewrites',
		disabled: 'Disabled: not contacted'
	};
	const blank = () => ({
		name: '',
		target: 'agent' as 'agent' | 'set',
		target_set_id: '',
		target_agent_id: '',
		tls_fingerprint: '',
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
	let f = $state(blank());
	let formError = $state('');
	let busy = $state(false);
	const editing = $derived(conn?.id ?? null);
	// Each opening starts from the connection (or empty for a new one).
	$effect(() => {
		if (!open) return;
		formError = '';
		f = conn
			? {
					...blank(),
					...conn,
					target: conn.target_agent_id ? 'agent' : 'set',
					target_set_id: conn.target_agent_id ? '' : conn.target_set_id,
					target_agent_id: conn.target_agent_id ?? '',
					port: String(conn.port),
					password: ''
				}
			: blank();
	});
	// Bug 90: HTTP has no certificate to check, so its fields go; the port follows the scheme while it is the default.
	// Through a Pulse agent only HTTPS is allowed (the password must reach the real instance).
	function schemeChanged(v: string) {
		if (f.port === (v === 'http' ? '443' : '80')) f.port = v === 'http' ? '80' : '443';
	}
	$effect(() => {
		if (f.target === 'agent' && f.scheme === 'http') {
			f.scheme = 'https';
			schemeChanged('https');
		}
	});
	async function save() {
		formError = '';
		busy = true;
		try {
			const body = {
				name: f.name,
				target_set_id: f.target === 'set' ? f.target_set_id : undefined,
				target_agent_id: f.target === 'agent' ? f.target_agent_id : undefined,
				tls_fingerprint: (f.scheme === 'https' && f.tls_fingerprint) || undefined,
				port: Number(f.port),
				scheme: f.scheme,
				path_prefix: f.path_prefix || undefined,
				tls_verify: f.scheme === 'https' && f.tls_verify,
				server_name: (f.scheme === 'https' && f.server_name) || undefined,
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
			if (row?.id) onsaved(row.id);
		} catch (e) {
			formError = problemText(e);
		} finally {
			busy = false;
		}
	}
</script>

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
				<Label for="ag-kind">Reached through</Label>
				<Select.Root type="single" bind:value={f.target}>
					<Select.Trigger id="ag-kind" class="w-full"
						>{f.target === 'agent' ? 'Pulse agent' : 'Address set'}</Select.Trigger
					>
					<Select.Content>
						<Select.Item value="agent">Pulse agent</Select.Item>
						<Select.Item value="set">Address set</Select.Item>
					</Select.Content>
				</Select.Root>
				{#if f.target === 'agent'}
					<Select.Root type="single" bind:value={f.target_agent_id}>
						<Select.Trigger aria-label="Pulse agent" class="w-full"
							>{pulseAgents.list.find((a) => a.id === f.target_agent_id)?.name ??
								'Choose an agent'}</Select.Trigger
						>
						<Select.Content>
							{#each pulseAgents.list as a (a.id)}<Select.Item value={a.id}>{a.name}</Select.Item
								>{/each}
						</Select.Content>
					</Select.Root>
				{:else}
					<Select.Root type="single" bind:value={f.target_set_id}>
						<Select.Trigger aria-label="Address set" class="w-full"
							>{targetNames.setName(f.target_set_id) ?? 'Choose a set'}</Select.Trigger
						>
						<Select.Content>
							{#each targetNames.sets as s (s.id)}<Select.Item value={s.id}>{s.name}</Select.Item
								>{/each}
						</Select.Content>
					</Select.Root>
					<TargetSources setId={f.target_set_id} />
				{/if}
			</div>
			<div class="grid grid-cols-[1fr_6rem] gap-3">
				<div class="space-y-1.5">
					<Label for="ag-scheme">Scheme</Label>
					<Select.Root type="single" bind:value={f.scheme} onValueChange={schemeChanged}>
						<Select.Trigger id="ag-scheme" class="w-full">{f.scheme.toUpperCase()}</Select.Trigger>
						<Select.Content>
							<Select.Item value="https">HTTPS</Select.Item>
							<Select.Item value="http" disabled={f.target === 'agent'}>HTTP</Select.Item>
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
			{#if f.scheme === 'https'}
				<div class="space-y-1.5">
					<Label for="ag-fp">Certificate fingerprint (SHA-256)</Label><Input
						id="ag-fp"
						class="font-mono"
						placeholder="For a self-signed certificate"
						bind:value={f.tls_fingerprint}
					/>
				</div>
				{#if !f.tls_fingerprint.trim()}<div class="space-y-1.5">
						<Label for="ag-host">Name on its certificate</Label><Input
							id="ag-host"
							bind:value={f.server_name}
							placeholder="ns.example.com"
						/>
					</div>
					<label class="flex items-center justify-between gap-3 text-md">
						<span>Check the certificate</span><Switch
							bind:checked={f.tls_verify}
							aria-label="Check the certificate"
						/>
					</label>{/if}
			{/if}
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
			<label class="flex items-center justify-between gap-3 text-md">
				<span>Save even if it does not answer yet</span><Switch
					bind:checked={f.save_inactive}
					aria-label="Save even if it does not answer yet"
				/>
			</label>
			{#if formError}<p role="alert" class="text-md text-bad-text">{formError}</p>{/if}
		</div>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			<Button variant="outline" onclick={() => (open = false)}>Cancel</Button>
			<Button
				onclick={save}
				disabled={busy ||
					!f.name.trim() ||
					!(f.target === 'agent' ? f.target_agent_id : f.target_set_id)}
				>{editing ? 'Save changes' : 'Add AdGuard'}</Button
			>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>
