<script lang="ts">
	// The parts of a service beyond its upstream: a stream's public port (04 §1.2) and the routing override (03 §5.1).
	import { onMount } from 'svelte';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import { Switch } from '#lib/components/ui/switch/index.ts';
	import { api } from '#lib/api/client.ts';
	import { isStream } from '#lib/services.ts';
	import type { FirewallHost } from '#lib/firewall.ts';

	let {
		kind,
		publicPort = $bindable(''),
		proxy = $bindable(false),
		entryHost = $bindable('')
	}: {
		kind: string;
		publicPort?: string;
		proxy?: boolean;
		entryHost?: string;
	} = $props();
	let hosts = $state<FirewallHost[]>([]);
	onMount(async () => {
		hosts = (await api.GET('/api/firewall/hosts')).data?.hosts ?? [];
	});
</script>

{#if isStream(kind) || hosts.length > 1}<div class="space-y-3 rounded-xl border p-3">
		{#if isStream(kind)}
			<div class="space-y-1.5">
				<Label for="sx-port">Public port</Label>
				<Input id="sx-port" bind:value={publicPort} inputmode="numeric" class="max-w-32" required />
				{#if publicPort}<p class="text-xs text-warn-text">
						Opening a port is a high-risk change: it waits for your approval.
					</p>{/if}
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
					class="h-10 w-full rounded-[10px] border bg-transparent px-3 text-md"
				>
					<option value="">The host it runs on</option>
					{#each hosts as h (h.id)}<option value={h.id}>{h.name}</option>{/each}
				</select>
			</div>
		{/if}
	</div>{/if}
