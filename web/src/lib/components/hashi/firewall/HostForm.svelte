<script lang="ts">
	// A new firewall host (02 §1 step 6, 05 §2): the host key is shown and pinned only after the admin compares it;
	// saving validates over SSH read-only. An external NAT host has nothing to log in to.
	import { untrack } from 'svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import { ShieldCheck } from '@lucide/svelte';
	import type { Schemas } from '$lib/api/client';
	import { lines, subnetOf, type FirewallInput } from '$lib/firewall';

	type Props = {
		traefikId: string;
		traefikHost: string;
		busy: boolean;
		scan: (host: string, port: number) => Promise<Schemas['HostKey'] | undefined>;
		save: (body: FirewallInput) => Promise<void>;
	};
	let { traefikId, traefikHost, busy, scan, save }: Props = $props();

	let form = $state({
		name: '',
		mode: 'managed' as FirewallInput['edge_mode'],
		host: '',
		port: 22,
		user: 'root',
		auth: 'hashi_key' as NonNullable<FirewallInput['auth']>,
		credential: '',
		passphrase: '',
		traefikIp: untrack(() => traefikHost),
		subnets: untrack(() => subnetOf(traefikHost)),
		publicIp: ''
	});
	let scanned = $state<Schemas['HostKey'] | null>(null);
	let rejected = $state(false);

	const body = (key?: string): FirewallInput => ({
		name: form.name.trim(),
		edge_mode: form.mode,
		host: form.mode === 'managed' ? form.host.trim() : undefined,
		port: form.mode === 'managed' ? Number(form.port) : undefined,
		user: form.mode === 'managed' ? form.user.trim() : undefined,
		auth: form.mode === 'managed' ? form.auth : undefined,
		credential: form.mode === 'managed' && form.auth !== 'hashi_key' ? form.credential : undefined,
		passphrase:
			form.mode === 'managed' && form.auth === 'private_key'
				? form.passphrase || undefined
				: undefined,
		host_key: key,
		traefik_connection_id: traefikId,
		internal_traefik_ip: form.traefikIp.trim(),
		subnets: lines(form.subnets),
		public_ip_override: form.publicIp.trim() || undefined,
		netbird_mode: 'disabled'
	});

	async function submit(e: Event) {
		e.preventDefault();
		rejected = false;
		if (form.mode === 'external_nat') return save(body());
		scanned = (await scan(form.host.trim(), Number(form.port))) ?? null;
	}
</script>

<form class="grid max-w-[62ch] gap-3 md:grid-cols-2" onsubmit={submit}>
	<div class="space-y-1.5">
		<Label for="fw-name">Name</Label>
		<Input id="fw-name" bind:value={form.name} placeholder="edge-a" class="h-10" required />
		<p class="text-[12px] text-muted-foreground">Becomes the host's DNS name.</p>
	</div>
	<div class="space-y-1.5">
		<Label for="fw-mode">How it is reached</Label>
		<select
			id="fw-mode"
			bind:value={form.mode}
			class="h-10 w-full rounded-[10px] border bg-transparent px-3 text-[13px]"
		>
			<option value="managed">Hashi manages its firewall over SSH</option>
			<option value="external_nat">Behind a router I configure myself</option>
		</select>
	</div>
	{#if form.mode === 'managed'}
		<div class="space-y-1.5">
			<Label for="fw-host">SSH address</Label>
			<Input id="fw-host" bind:value={form.host} class="h-10" required />
		</div>
		<div class="grid grid-cols-[6rem_1fr] gap-2">
			<div class="space-y-1.5">
				<Label for="fw-port">Port</Label>
				<Input id="fw-port" bind:value={form.port} inputmode="numeric" class="h-10" />
			</div>
			<div class="space-y-1.5">
				<Label for="fw-user">User</Label>
				<Input id="fw-user" bind:value={form.user} class="h-10" required />
			</div>
		</div>
		<div class="space-y-1.5">
			<Label for="fw-auth">Login</Label>
			<select
				id="fw-auth"
				bind:value={form.auth}
				class="h-10 w-full rounded-[10px] border bg-transparent px-3 text-[13px]"
			>
				<option value="hashi_key">Hashi's SSH key</option>
				<option value="private_key">A private key</option>
				<option value="password">A password</option>
			</select>
		</div>
		{#if form.auth === 'password'}
			<div class="space-y-1.5">
				<Label for="fw-cred">Password</Label>
				<Input
					id="fw-cred"
					type="password"
					bind:value={form.credential}
					autocomplete="off"
					class="h-10"
					required
				/>
				<p class="text-[12px] text-muted-foreground">Stored encrypted, never shown again.</p>
			</div>
		{:else if form.auth === 'private_key'}
			<div class="space-y-1.5 md:col-span-2">
				<Label for="fw-cred">Private key</Label>
				<textarea
					id="fw-cred"
					bind:value={form.credential}
					rows="4"
					autocomplete="off"
					required
					class="w-full rounded-[10px] border bg-transparent px-3 py-2 font-mono text-[12.5px]"
				></textarea>
				<p class="text-[12px] text-muted-foreground">Stored encrypted, never shown again.</p>
			</div>
			<div class="space-y-1.5">
				<Label for="fw-pass">Passphrase (if any)</Label>
				<Input
					id="fw-pass"
					type="password"
					bind:value={form.passphrase}
					autocomplete="off"
					class="h-10"
				/>
			</div>
		{/if}
	{:else}
		<div class="space-y-1.5">
			<Label for="fw-pub">Public address</Label>
			<Input id="fw-pub" bind:value={form.publicIp} class="h-10" required />
		</div>
	{/if}
	<div class="space-y-1.5">
		<Label for="fw-tip">Traefik's address on this network</Label>
		<Input id="fw-tip" bind:value={form.traefikIp} class="h-10" required />
		<p class="text-[12px] text-muted-foreground">Public traffic is forwarded here.</p>
	</div>
	<div class="space-y-1.5">
		<Label for="fw-sub">Networks behind this host</Label>
		<Input id="fw-sub" bind:value={form.subnets} placeholder="10.0.4.0/24" class="h-10" required />
		<p class="text-[12px] text-muted-foreground">
			Services with a target in one of these run on this host.
		</p>
	</div>
	<div class="md:col-span-2">
		<Button type="submit" disabled={busy}>
			{#if form.mode === 'managed'}{busy ? 'Connecting…' : 'Read the host key'}{:else}Save{/if}
		</Button>
	</div>
</form>

{#if scanned}
	<div class="mt-5 max-w-[62ch] rounded-2xl border border-primary/40 bg-primary/[0.06] p-4">
		<div class="flex items-center gap-2 text-[13.5px] font-medium">
			<ShieldCheck class="size-4" />Confirm this host key
		</div>
		<p class="mt-1 text-[13px] text-muted-foreground">
			First contact with {form.host}. Compare it with what the host shows.
		</p>
		<Mono class="mt-3 block rounded-lg bg-surface-2 px-3 py-2"
			>{scanned.type} {scanned.fingerprint}</Mono
		>
		<div class="mt-3 flex gap-2">
			<Button size="sm" disabled={busy} onclick={() => save(body(scanned?.key))}>
				{busy ? 'Checking the host…' : 'It matches, pin it'}
			</Button>
			<Button size="sm" variant="ghost" onclick={() => ((scanned = null), (rejected = true))}
				>It doesn't match</Button
			>
		</div>
	</div>
{:else if rejected}
	<p
		role="alert"
		class="mt-5 max-w-[62ch] rounded-2xl border border-bad/30 bg-bad/10 p-4 text-[13px] text-bad-text"
	>
		Connection stopped. A different key can mean the wrong machine or an interception. Check the
		address and try again.
	</p>
{/if}
