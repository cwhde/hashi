<script lang="ts">
	import SshTarget from '#lib/components/hashi/SshTarget.svelte';
	// Add a firewall host, or change how Hashi reaches one (05 §2, bug 24). What belongs to the host itself (networks,
	// NetBird, ports, host access) stays on its page. A new address or key is validated and confirmed again there.
	import { goto } from '$app/navigation';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import * as Sheet from '#lib/components/ui/sheet/index.ts';
	import Mono from '#lib/components/hashi/Mono.svelte';
	import HostForm from '#lib/components/hashi/firewall/HostForm.svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { withReauth } from '#lib/auth/reauth.ts';
	import { firewallInput, type FirewallHost, type FirewallInput } from '#lib/firewall.ts';

	let {
		open = $bindable(false),
		host = null,
		onsaved
	}: { open?: boolean; host?: FirewallHost | null; onsaved: () => void } = $props();

	let conns = $state<Schemas['TraefikConnection'][]>([]);
	let hosts = $state<FirewallHost[]>([]);
	let publicKey = $state('');
	let busy = $state(false);
	let target = $state<SshTarget>();
	let error = $state('');
	let f = $state({ name: '', host: '', port: '22', user: 'root' });
	let key = $state<Schemas['HostKey'] | null>(null);
	$effect(() => {
		if (!open) return;
		error = '';
		key = null;
		if (host) f = { name: host.name, host: host.host, port: String(host.port), user: host.user };
		void (async () => {
			publicKey = (await api.POST('/api/firewall/identity')).data?.public_key ?? '';
			const [r, h] = await Promise.all([
				api.GET('/api/traefik/connections'),
				api.GET('/api/firewall/hosts')
			]);
			conns = r.data?.connections ?? [];
			hosts = h.data?.hosts ?? [];
		})();
	});

	// ---- add (HostForm does the steps) ----
	async function run<T>(fn: () => Promise<T>): Promise<T | undefined> {
		error = '';
		busy = true;
		try {
			return await fn();
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = false;
		}
	}
	const install = async (body: Schemas['KeyInstallInput']) =>
		!!(await run(async () => {
			await withReauth(() => api.POST('/api/firewall/install-key', { body }));
			return true;
		}));
	const scanFor = (h: string, port: number) =>
		run(async () => {
			const r = await api.POST('/api/firewall/scan', { body: { host: h, port } });
			if (r.error || !r.data) throw r.error;
			return r.data;
		});
	async function create(body: FirewallInput) {
		await run(async () => {
			const r = await api.POST('/api/firewall/hosts', { body });
			if (r.error || !r.data) throw r.error;
			open = false;
			onsaved();
			await goto(`/firewall/${r.data.id}`); // validation and the ownership step continue there
		});
	}

	// ---- edit ----
	const managed = $derived(host?.edge_mode === 'managed');
	const moved = $derived(
		!!host && managed && (f.host.trim() !== host.host || Number(f.port) !== host.port)
	);
	async function scan() {
		key = (await scanFor(f.host.trim(), Number(f.port))) ?? null;
	}
	async function save() {
		const h = host;
		if (!h) return;
		await run(async () => {
			if (target?.dirty() && !(await target.commit())) return;
			const body: FirewallInput = {
				...firewallInput(h),
				name: f.name.trim(),
				host: managed ? f.host.trim() : undefined,
				port: managed ? Number(f.port) : undefined,
				user: managed ? f.user.trim() : undefined,
				host_key: key?.key
			};
			const r = await api.PUT('/api/firewall/hosts/{id}', {
				params: { path: { id: h.id } },
				body
			});
			if (r.error) throw r.error;
			open = false;
			onsaved();
		});
	}
</script>

<Sheet.Root bind:open>
	<Sheet.Content class="w-full sm:max-w-2xl">
		<Sheet.Header>
			<Sheet.Title>{host ? `Edit ${host.name}` : 'Add a firewall host'}</Sheet.Title>
		</Sheet.Header>
		<div class="space-y-3 overflow-y-auto px-4 pb-4 text-md">
			{#if error}<p role="alert" class="text-bad-text">{error}</p>{/if}
			{#if !host}
				{#if conns.length}
					<HostForm {conns} {hosts} {busy} scan={scanFor} save={create} {publicKey} {install} />
				{:else}
					<p class="text-muted-foreground">Add a Traefik host first.</p>
				{/if}
			{:else}
				<div class="space-y-1.5">
					<Label for="fh-name">Name</Label><Input id="fh-name" bind:value={f.name} />
				</div>
				{#if managed}
					<div class="grid grid-cols-[minmax(0,1fr)_5rem] gap-2">
						<div class="space-y-1.5">
							<Label for="fh-host">SSH address</Label><Input
								id="fh-host"
								class="font-mono"
								bind:value={f.host}
								oninput={() => (key = null)}
							/>
						</div>
						<div class="space-y-1.5">
							<Label for="fh-port">Port</Label><Input
								id="fh-port"
								bind:value={f.port}
								inputmode="numeric"
								oninput={() => (key = null)}
							/>
						</div>
					</div>
					<div class="space-y-1.5">
						<Label for="fh-user">User</Label><Input id="fh-user" bind:value={f.user} />
					</div>
					{#key host.id}<SshTarget
							bind:this={target}
							kind="firewall"
							id={host.id}
							value={host.ssh_target_set_id}
							agent={host.ssh_target_agent_id}
						/>{/key}
					{#if moved}
						<Button size="sm" variant="outline" onclick={scan} disabled={busy}
							>Read its SSH key</Button
						>
						{#if key}<p>
								The host says it is <Mono class="break-all">{key.fingerprint}</Mono>.
								{key.fingerprint === host.host_key_fingerprint
									? 'The same machine.'
									: 'Not the key Hashi knows: another machine, whose ruleset is read again.'}
							</p>{/if}
					{/if}
				{/if}
			{/if}
		</div>
		{#if host}
			<Sheet.Footer class="flex-row justify-end gap-2 border-t">
				<Button variant="outline" onclick={() => (open = false)}>Cancel</Button>
				<Button onclick={save} disabled={busy || !f.name.trim() || (moved && !key)}
					>{moved ? 'Trust this key and save' : 'Save changes'}</Button
				>
			</Sheet.Footer>
		{/if}
	</Sheet.Content>
</Sheet.Root>
