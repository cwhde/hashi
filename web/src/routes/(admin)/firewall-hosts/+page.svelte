<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import type { FirewallHost } from '$lib/api/types';
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import PanelSection from '$lib/components/layout/PanelSection.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { Shield } from 'lucide-svelte';

	let script = $state('');
	let hosts = $state<FirewallHost[]>([]);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let form = $state({
		name: 'edge-firewall',
		domain: '',
		managedSubnets: '192.168.0.0/16',
		linkedTraefikHost: '',
		internalTraefikIp: '',
		publicIp: '',
		wanInterface: '',
		netBirdEnabled: true,
		netBirdInterface: 'wt0',
		netBirdOverlayCidrs: '100.110.0.0/16',
		netBirdRoutedCidrs: '',
		netBirdRoutingPeer: false,
		rollbackTimerSeconds: 300
	});

	$effect(() => {
		void loadHosts();
	});

	async function loadHosts() {
		try {
			hosts = await api.listFirewallHosts();
		} catch {
			hosts = [];
		}
	}

	async function render() {
		loading = true;
		error = null;
		try {
			const result = await api.renderFirewall({
				name: form.name,
				domain: form.domain,
				managedSubnets: form.managedSubnets.split(',').map((s) => s.trim()),
				linkedTraefikHost: form.linkedTraefikHost,
				internalTraefikIp: form.internalTraefikIp,
				publicIp: form.publicIp || null,
				wanInterface: form.wanInterface || null,
				netBirdEnabled: form.netBirdEnabled,
				netBirdInterface: form.netBirdInterface,
				netBirdOverlayCidrs: form.netBirdOverlayCidrs
					.split(',')
					.map((s) => s.trim())
					.filter(Boolean),
				netBirdRoutedCidrs: form.netBirdRoutedCidrs
					.split(',')
					.map((s) => s.trim())
					.filter(Boolean),
				netBirdRoutingPeer: form.netBirdRoutingPeer,
				rollbackTimerSeconds: form.rollbackTimerSeconds
			});
			script = result.script;
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to render firewall script';
		} finally {
			loading = false;
		}
	}
</script>

<AdminSectionPage
	title="Firewall Hosts"
	description="Linux edge hosts, managed subnets, port forwards, and Hashi chains."
	icon={Shield}
>
	{#if hosts.length > 0}
		<PanelSection title="Configured hosts" description="Persisted firewall host records.">
			<ul class="space-y-2 text-sm">
				{#each hosts as host (host.id)}
					<li class="rounded-md border border-border px-3 py-2">
						<div class="font-medium text-white">{host.name}</div>
						<div class="text-xs text-muted-foreground">
							{host.internalTraefikIp} · NetBird {host.netBirdEnabled ? 'enabled' : 'disabled'} ·
							{host.netBirdDetected ? 'detected on last apply' : 'not detected yet'}
						</div>
					</li>
				{/each}
			</ul>
		</PanelSection>
	{/if}

	<PanelSection title="Render firewall script" description="Preview generated iptables script for a host profile.">
		<div class="grid max-w-xl gap-3">
			<div class="grid gap-1.5">
				<Label for="fw-name">Host name</Label>
				<Input id="fw-name" bind:value={form.name} />
			</div>
			<div class="grid gap-1.5">
				<Label for="fw-domain">Domain (FQDN or zone for trusted IP resolution)</Label>
				<Input id="fw-domain" bind:value={form.domain} />
			</div>
			<div class="grid gap-1.5">
				<Label for="fw-subnets">Managed subnets (comma-separated)</Label>
				<Input id="fw-subnets" bind:value={form.managedSubnets} />
			</div>
			<div class="grid grid-cols-2 gap-3">
				<div class="grid gap-1.5">
					<Label for="fw-traefik">Linked Traefik host</Label>
					<Input id="fw-traefik" bind:value={form.linkedTraefikHost} />
				</div>
				<div class="grid gap-1.5">
					<Label for="fw-ip">Internal Traefik IP</Label>
					<Input id="fw-ip" bind:value={form.internalTraefikIp} />
				</div>
			</div>
			<div class="grid grid-cols-2 gap-3">
				<div class="grid gap-1.5">
					<Label for="fw-public-ip">Public IP override</Label>
					<Input id="fw-public-ip" bind:value={form.publicIp} placeholder="Resolve from name.domain if empty" />
				</div>
				<div class="grid gap-1.5">
					<Label for="fw-wan">WAN interface override</Label>
					<Input id="fw-wan" bind:value={form.wanInterface} />
				</div>
			</div>
			<div class="grid gap-2 rounded-md border border-border p-3">
				<div class="flex items-center gap-2">
					<Switch bind:checked={form.netBirdEnabled} id="fw-netbird-enabled" />
					<Label for="fw-netbird-enabled">NetBird compatibility rules</Label>
				</div>
				<div class="grid grid-cols-2 gap-3">
					<div class="grid gap-1.5">
						<Label for="fw-netbird-if">NetBird interface</Label>
						<Input id="fw-netbird-if" bind:value={form.netBirdInterface} />
					</div>
					<div class="grid gap-1.5">
						<Label for="fw-netbird-overlay">Overlay CIDRs</Label>
						<Input id="fw-netbird-overlay" bind:value={form.netBirdOverlayCidrs} />
					</div>
				</div>
				<div class="grid gap-1.5">
					<Label for="fw-netbird-routed">Routed CIDRs (routing peer)</Label>
					<Input id="fw-netbird-routed" bind:value={form.netBirdRoutedCidrs} />
				</div>
				<div class="flex items-center gap-2">
					<Switch bind:checked={form.netBirdRoutingPeer} id="fw-netbird-routing-peer" />
					<Label for="fw-netbird-routing-peer">Act as NetBird routing peer</Label>
				</div>
			</div>
			<Button onclick={() => render()} disabled={loading}>
				{loading ? 'Rendering…' : 'Render script'}
			</Button>
		</div>
	</PanelSection>

	{#if error}<p class="text-sm text-destructive">{error}</p>{/if}
	{#if script}
		<PanelSection title="Generated script" description="Review before apply via sync workflow.">
			<pre
				class="max-h-[32rem] overflow-auto rounded-md border border-border bg-hashi-bg-dark p-3 font-mono text-[11px]">{script}</pre>
		</PanelSection>
	{/if}
</AdminSectionPage>
