<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
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
	let connectionId = $state<string | null>(null);
	let error = $state<string | null>(null);
	let previewRecords = $state<Array<{ name: string; type: string; ttl: number; value: string }>>([]);

	async function validateConnection() {
		validating = true;
		error = null;
		try {
			const result = (await api.validateHetznerDnsProvider({ apiToken })) as {
				valid?: boolean;
				error?: string | null;
			};
			if (!result.valid) {
				validated = false;
				error = result.error ?? 'Provider token validation failed.';
				return;
			}
			validated = true;
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Validation failed';
			validated = false;
		} finally {
			validating = false;
		}
	}

	async function saveProvider() {
		creating = true;
		error = null;
		try {
			const created = (await api.createHetznerDnsConnection({
				name: 'setup-hetzner',
				apiToken,
				zoneName: zone,
				defaultTtl
			})) as { id?: string };
			connectionId = created.id ?? null;
			if (connectionId) {
				const records = await api.listProviderDnsRecords(connectionId);
				previewRecords = records.slice(0, 50).map((record) => ({
					name: record.name ?? '',
					type: record.type ?? '',
					ttl: Number(record.ttl ?? 0),
					value: record.value ?? ''
				}));
			}
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to save DNS connection';
		} finally {
			creating = false;
		}
	}

	async function continueToNext() {
		creating = true;
		error = null;
		try {
			if (!connectionId) {
				await saveProvider();
			}
			if (connectionId) {
				await oncomplete();
			}
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to advance setup';
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

	{#if previewRecords.length > 0}
		<div>
			<p class="mb-2 text-xs text-muted-foreground">
				Showing first {previewRecords.length} zone records (import selection available on the DNS admin page).
			</p>
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
						{#each previewRecords as record (record.name + record.type + record.value)}
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
		</div>
	{/if}

	<div class="flex gap-2">
		<Button variant="outline" disabled={validating || !apiToken} onclick={() => validateConnection()}>
			{validating ? 'Validating…' : 'Validate connection'}
		</Button>
		{#if connectionId}
			<Button onclick={() => continueToNext()} disabled={advancing || creating}>
				{creating ? 'Continuing…' : 'Continue'}
			</Button>
		{:else}
			<Button
				onclick={() => saveProvider()}
				disabled={advancing || creating || !validated || !zone || !apiToken}
			>
				{creating ? 'Saving…' : 'Save & preview records'}
			</Button>
		{/if}
	</div>
</div>
