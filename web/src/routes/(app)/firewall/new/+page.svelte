<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import HostForm from '$lib/components/hashi/firewall/HostForm.svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import type { FirewallInput } from '$lib/firewall';

	let conn = $state<Schemas['TraefikConnection'] | null>(null);
	let busy = $state(false);
	let error = $state('');
	onMount(async () => {
		const r = await api.GET('/api/traefik/connections');
		conn = r.data?.connections[0] ?? null;
	});
	async function scan(host: string, port: number) {
		error = '';
		busy = true;
		try {
			const r = await api.POST('/api/firewall/scan', { body: { host, port } });
			if (r.error || !r.data) throw r.error;
			return r.data;
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = false;
		}
	}
	async function save(body: FirewallInput) {
		error = '';
		busy = true;
		try {
			const r = await api.POST('/api/firewall/hosts', { body });
			if (r.error || !r.data) throw r.error;
			await goto(`/firewall/${r.data.id}`);
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = false;
		}
	}
</script>

<PageHeader
	title="Add firewall host"
	description="Nothing changes on the host until you approve a plan."
/>
{#if error}<p
		role="alert"
		class="mb-4 rounded-2xl border border-bad/30 bg-bad/10 p-3 text-[13px] text-bad-text"
	>
		{error}
	</p>{/if}
{#if conn}
	<HostForm traefikId={conn.id} traefikHost={conn.host} {busy} {scan} {save} />
{:else}
	<p class="text-[13px] text-muted-foreground">
		Connect a Traefik host first: public traffic is forwarded to it.
	</p>
{/if}
