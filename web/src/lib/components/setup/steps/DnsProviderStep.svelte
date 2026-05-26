<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import ApiPendingBanner from '$lib/components/layout/ApiPendingBanner.svelte';
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
	let validating = $state(false);
	let creating = $state(false);
	let validated = $state(false);
	let error = $state<string | null>(null);
	let previewRecords = $state<Array<{ name: string; type: string; ttl: number; value: string }>>([]);

	async function validateConnection() {
		validating = true;
		error = null;
		try {
			await api.validateHetznerDnsProvider({ apiToken });
			validated = true;
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Validation failed';
			validated = false;
		} finally {
			validating = false;
		}
	}

	async function saveAndContinue() {
		creating = true;
		error = null;
		try {
			await api.createHetznerDnsConnection({
				name: 'setup-hetzner',
				apiToken,
				zoneName: zone,
				defaultTtl
			});
			await oncomplete();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to save DNS connection';
		} finally {
			creating = false;
		}
	}
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

	{#if error}
		<p class="text-xs text-destructive">{error}</p>
	{/if}
	{#if validated}
		<p class="text-xs text-emerald-300">Provider token validated.</p>
	{/if}

	<ApiPendingBanner
		message="Record import preview"
		detail="Use the DNS admin page after setup for import preview and sync plan/apply workflows."
	/>

	{#if previewRecords.length > 0}
		<div class="overflow-hidden rounded-md border border-border">
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>Name</TableHead>
						<TableHead>Type</TableHead>
						<TableHead>TTL</TableHead>
						<TableHead>Value</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{#each previewRecords as record}
						<TableRow>
							<TableCell class="font-mono text-xs">{record.name}</TableCell>
							<TableCell>{record.type}</TableCell>
							<TableCell>{record.ttl}</TableCell>
							<TableCell class="max-w-[12rem] truncate font-mono text-xs">{record.value}</TableCell>
						</TableRow>
					{/each}
				</TableBody>
			</Table>
		</div>
	{/if}

	<div class="flex gap-2">
		<Button variant="outline" disabled={validating || !apiToken} onclick={() => validateConnection()}>
			{validating ? 'Validating…' : 'Validate connection'}
		</Button>
		<Button
			onclick={() => saveAndContinue()}
			disabled={advancing || creating || !validated || !zone || !apiToken}
		>
			{creating ? 'Saving…' : 'Save & continue'}
		</Button>
	</div>
</div>
