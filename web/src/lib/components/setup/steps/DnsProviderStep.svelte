<script lang="ts">
	import ApiPendingBanner, {
		apiUnavailable
	} from '$lib/components/layout/ApiPendingBanner.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import {
		Table,
		TableBody,
		TableCell,
		TableHead,
		TableHeader,
		TableRow
	} from '$lib/components/ui/table';
	import { Checkbox } from '$lib/components/ui/checkbox';

	let {
		oncomplete,
		advancing = false
	}: {
		oncomplete: () => void | Promise<void>;
		advancing?: boolean;
	} = $props();

	let provider = $state('hetzner');
	let apiToken = $state('');
	let zone = $state('');
	let defaultTtl = $state(300);
	let dryRunAllowed = $state(false);

	const sampleRecords = [
		{ name: 'hashi', type: 'A', ttl: 300, value: '203.0.113.10' },
		{ name: 'www', type: 'CNAME', ttl: 300, value: 'hashi.example.com' },
		{ name: '_hashi-test', type: 'TXT', ttl: 60, value: 'validation' }
	];
</script>

<div class="grid max-w-2xl gap-4">
	<div class="grid gap-1.5">
		<Label for="dns-provider">Provider</Label>
		<Input id="dns-provider" bind:value={provider} readonly />
	</div>
	<div class="grid gap-1.5">
		<Label for="dns-token">API token</Label>
		<Input id="dns-token" type="password" bind:value={apiToken} placeholder="••••••••" />
	</div>
	<div class="grid grid-cols-2 gap-3">
		<div class="grid gap-1.5">
			<Label for="dns-zone">Zone / domain</Label>
			<Input id="dns-zone" bind:value={zone} placeholder="example.com" />
		</div>
		<div class="grid gap-1.5">
			<Label for="dns-ttl">Default TTL</Label>
			<Input id="dns-ttl" type="number" bind:value={defaultTtl} />
		</div>
	</div>

	<div class="flex items-center gap-2 text-xs">
		<Checkbox bind:checked={dryRunAllowed} id="dns-dryrun" />
		<Label for="dns-dryrun">Allow harmless `_hashi-test` dry-run write validation</Label>
	</div>

	<ApiPendingBanner
		message={apiUnavailable('DNS provider validation')}
		detail="POST /api/setup/dns/validate will read zones, list records, and optionally test writes."
	/>

	<div class="space-y-2">
		<p class="text-xs font-medium text-hashi-contrast">Import preview (sample)</p>
		<div class="overflow-hidden rounded-md border border-border">
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead class="w-8"></TableHead>
						<TableHead>Name</TableHead>
						<TableHead>Type</TableHead>
						<TableHead>TTL</TableHead>
						<TableHead>Value</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{#each sampleRecords as record}
						<TableRow>
							<TableCell><Checkbox checked={record.name !== '_hashi-test'} /></TableCell>
							<TableCell class="font-mono text-xs">{record.name}</TableCell>
							<TableCell>{record.type}</TableCell>
							<TableCell>{record.ttl}</TableCell>
							<TableCell class="max-w-[12rem] truncate font-mono text-xs">{record.value}</TableCell>
						</TableRow>
					{/each}
				</TableBody>
			</Table>
		</div>
		<p class="text-[11px] text-muted-foreground">
			NS and SOA records are never shown for pruning. Destructive prune requires confirmation.
		</p>
	</div>

	<div class="flex gap-2">
		<Button variant="outline" disabled>Validate connection</Button>
		<Button onclick={() => oncomplete()} disabled={advancing}>Save & continue</Button>
	</div>
</div>
