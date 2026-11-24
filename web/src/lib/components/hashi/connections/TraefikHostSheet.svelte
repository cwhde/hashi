<script lang="ts">
	// Add or edit how Hashi reaches a Traefik host over SSH (04 §5, bug 24). A new address needs its key read and
	// compared; a key that differs from the pinned one is another machine, which Hashi validates and takes over again.
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import * as Sheet from '#lib/components/ui/sheet/index.ts';
	import Mono from '#lib/components/hashi/Mono.svelte';
	import SshTarget from '#lib/components/hashi/SshTarget.svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { withReauth } from '#lib/auth/reauth.ts';

	type Conn = Schemas['TraefikConnection'];
	let {
		open = $bindable(false),
		conn = null,
		onsaved
	}: { open?: boolean; conn?: Conn | null; onsaved: (row: Conn) => void } = $props();

	let f = $state({ name: '', host: '', port: '22', user: 'root' });
	let key = $state<Schemas['HostKey'] | null>(null);
	let identity = $state('');
	let error = $state('');
	let busy = $state(false);
	let target = $state<SshTarget>();
	$effect(() => {
		if (!open) return;
		f = conn
			? { name: conn.name, host: conn.host, port: String(conn.port), user: conn.user }
			: { name: '', host: '', port: '22', user: 'root' };
		key = null;
		error = '';
		void api.GET('/api/traefik/identity').then((r) => (identity = r.data?.public_key ?? ''));
	});
	const moved = $derived(!conn || f.host.trim() !== conn.host || Number(f.port) !== conn.port);
	const otherMachine = $derived(!!conn && !!key && key.fingerprint !== conn.host_key_fingerprint);
	// The key is read whenever the address is new; it is never needed for a rename.
	const ready = $derived(!!f.name.trim() && !!f.host.trim() && (!moved || !!key));

	async function scan() {
		error = '';
		key = null;
		const r = await api.POST('/api/traefik/scan', {
			body: { host: f.host.trim(), port: Number(f.port) }
		});
		if (r.error) error = problemText(r.error);
		else key = r.data;
	}
	async function save() {
		error = '';
		busy = true;
		try {
			if (target?.dirty() && !(await target.commit())) return;
			const body: Schemas['TraefikConnectionInput'] = {
				name: f.name.trim(),
				host: f.host.trim(),
				port: Number(f.port),
				user: f.user.trim(),
				auth: conn?.auth ?? 'hashi_key',
				host_key: key?.key,
				// An edit keeps what this drawer does not show.
				internal_ip: conn?.internal_ip,
				traefik_binary: conn?.traefik_binary,
				config_dir: conn?.config_dir,
				api_port: conn?.api_port
			};
			const id = conn?.id;
			const row = await withReauth(() =>
				id
					? api.PUT('/api/traefik/hosts/{id}', { params: { path: { id } }, body })
					: api.POST('/api/traefik/hosts', { body })
			);
			if (!row) return;
			open = false;
			onsaved(row);
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
			<Sheet.Title>{conn ? `Edit ${conn.name}` : 'Add a Traefik host'}</Sheet.Title>
		</Sheet.Header>
		<div class="space-y-3 overflow-y-auto px-4 text-md">
			<div class="space-y-1.5">
				<Label for="th-name">Name</Label><Input id="th-name" bind:value={f.name} />
			</div>
			<div class="grid grid-cols-[minmax(0,1fr)_5rem] gap-2">
				<div class="space-y-1.5">
					<Label for="th-host">SSH address</Label><Input
						id="th-host"
						class="font-mono"
						bind:value={f.host}
						oninput={() => (key = null)}
					/>
				</div>
				<div class="space-y-1.5">
					<Label for="th-port">Port</Label><Input
						id="th-port"
						bind:value={f.port}
						inputmode="numeric"
						oninput={() => (key = null)}
					/>
				</div>
			</div>
			<div class="space-y-1.5">
				<Label for="th-user">User</Label><Input id="th-user" bind:value={f.user} />
			</div>
			{#if conn}{#key conn.id}<SshTarget
						bind:this={target}
						kind="traefik"
						id={conn.id}
						value={conn.ssh_target_set_id}
					/>{/key}{/if}
			{#if moved}
				{#if identity}<div class="space-y-1">
						<p>Put Hashi's key in this user's authorized keys:</p>
						<Mono class="block text-sm break-all">{identity}</Mono>
					</div>{/if}
				<Button size="sm" variant="outline" onclick={scan} disabled={!f.host.trim()}
					>Read its SSH key</Button
				>
				{#if key}<p>
						The host says it is <Mono class="break-all">{key.fingerprint}</Mono>.
					</p>{/if}
				{#if otherMachine}<p role="status" class="text-warn-text">
						This is not the key Hashi knows, so it is another machine: Hashi checks it and takes it
						over again.
					</p>{/if}
			{/if}
			{#if error}<p role="alert" class="text-bad-text">{error}</p>{/if}
		</div>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			<Button variant="outline" onclick={() => (open = false)}>Cancel</Button>
			<Button onclick={save} disabled={busy || !ready}
				>{moved ? 'Trust this key and save' : 'Save changes'}</Button
			>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>
