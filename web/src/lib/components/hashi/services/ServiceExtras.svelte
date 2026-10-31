<script lang="ts">
	// The parts of a service that reach beyond one address: following an address set (05a §5.2), a stream's public port
	// (04 §1.2) and the routing override (03 §5.1).
	import { onMount } from 'svelte';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { Button } from '$lib/components/ui/button';
	import IpSetPicker from '$lib/components/hashi/ipsets/IpSetPicker.svelte';
	import { api } from '$lib/api/client';
	import { listSets, type IPSet } from '$lib/ipsets';
	import { isStream } from '$lib/services';
	import type { FirewallHost } from '$lib/firewall';

	let {
		kind,
		setId = $bindable(''),
		publicPort = $bindable(''),
		proxy = $bindable(false),
		entryHost = $bindable('')
	}: {
		kind: string;
		setId?: string;
		publicPort?: string;
		proxy?: boolean;
		entryHost?: string;
	} = $props();
	let sets = $state<IPSet[]>([]);
	let hosts = $state<FirewallHost[]>([]);
	onMount(async () => {
		sets = await listSets().catch(() => []);
		hosts = (await api.GET('/api/firewall/hosts')).data?.hosts ?? [];
	});
</script>

<div class="space-y-3 rounded-xl border p-3">
	<div class="space-y-1.5">
		<Label for="sx-set">Follow an IP set instead</Label>
		<div class="flex gap-2">
			<IpSetPicker bind:value={setId} {sets} addressOnly id="sx-set" />
			{#if setId}<Button variant="ghost" size="sm" onclick={() => (setId = '')}>Clear</Button>{/if}
		</div>
		<p class="text-[12px] text-muted-foreground">
			Every address in the set gets traffic. If the set is ever empty, the last addresses stay.
		</p>
	</div>
	{#if isStream(kind)}
		<div class="space-y-1.5">
			<Label for="sx-port">Public port</Label>
			<Input id="sx-port" bind:value={publicPort} inputmode="numeric" class="max-w-32" required />
			<p class="text-[12px] text-muted-foreground">
				Opening a port is a high-risk change: it waits for your approval.
			</p>
		</div>
		{#if kind === 'tcp'}
			<div class="flex items-center justify-between gap-3">
				<Label for="sx-proxy">Send the visitor's address with PROXY protocol</Label><Switch
					id="sx-proxy"
					bind:checked={proxy}
				/>
			</div>
		{/if}
	{/if}
	{#if hosts.length > 1}
		<div class="space-y-1.5">
			<Label for="sx-entry">Public traffic enters through</Label>
			<select
				id="sx-entry"
				bind:value={entryHost}
				class="h-10 w-full rounded-[10px] border bg-transparent px-3 text-[13px]"
			>
				<option value="">The host it runs on</option>
				{#each hosts as h (h.id)}<option value={h.id}>{h.name}</option>{/each}
			</select>
		</div>
	{/if}
</div>
