<script lang="ts">
	import { goto } from '$app/navigation';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import * as Select from '#lib/components/ui/select/index.ts';
	import * as Sheet from '#lib/components/ui/sheet/index.ts';
	import { api, problemText } from '#lib/api/client.ts';

	let { open = $bindable(false), onadded }: { open?: boolean; onadded: () => void } = $props();

	const types: Record<string, { label: string; hint: string; placeholder: string }> = {
		https: {
			label: 'HTTPS',
			hint: '',
			placeholder: 'https://app.example.com/'
		},
		http: {
			label: 'HTTP',
			hint: '',
			placeholder: 'http://192.0.2.10:8080/'
		},
		h2c: {
			label: 'HTTP/2 without TLS',
			hint: '',
			placeholder: 'http://192.0.2.10:8080/'
		},
		tcp: {
			label: 'TCP connect',
			hint: '',
			placeholder: '192.0.2.10:22'
		},
		udp: {
			label: 'UDP',
			hint: '',
			placeholder: '192.0.2.10:53'
		},
		dns: {
			label: 'DNS',
			hint: '',
			placeholder: 'app.example.com or app.example.com@192.0.2.53:53'
		},
		tls: {
			label: 'Certificate expiry',
			hint: 'Warns 21 days before expiry, fails 7 days before.',
			placeholder: 'app.example.com:443'
		}
	};
	let name = $state('');
	let group = $state('');
	let type = $state('https');
	let target = $state('');
	let probe = $state('');
	let expect = $state('');
	let error = $state('');
	let busy = $state(false);

	async function save() {
		error = '';
		busy = true;
		try {
			const r = await api.POST('/api/monitor/endpoints', {
				body: { name, group: group || undefined, type, target, probe, expect }
			});
			if (r.error || !r.data) throw r.error;
			open = false;
			onadded();
			goto(`/status/${r.data.id}`);
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = false;
		}
	}
</script>

<Sheet.Root bind:open>
	<Sheet.Content class="w-full sm:max-w-md">
		<Sheet.Header>
			<Sheet.Title>Add check</Sheet.Title>
		</Sheet.Header>
		<div class="space-y-4 overflow-y-auto px-4">
			<div class="space-y-1.5">
				<Label for="ck-name">Name</Label><Input id="ck-name" bind:value={name} />
			</div>
			<div class="space-y-1.5">
				<Label for="ck-group">Group</Label><Input
					id="ck-group"
					bind:value={group}
					placeholder="My checks"
				/>
			</div>
			<div class="space-y-1.5">
				<Label for="ck-type">Type</Label>
				<Select.Root type="single" bind:value={type}>
					<Select.Trigger id="ck-type" class="w-full">{types[type]!.label}</Select.Trigger>
					<Select.Content>
						{#each Object.entries(types) as [v, t] (v)}<Select.Item value={v}>{t.label}</Select.Item
							>{/each}
					</Select.Content>
				</Select.Root>
				{#if types[type]!.hint}<p class="text-xs text-muted-foreground">{types[type]!.hint}</p>{/if}
			</div>
			<div class="space-y-1.5">
				<Label for="ck-target">Target</Label>
				<Input id="ck-target" bind:value={target} placeholder={types[type]!.placeholder} />
			</div>
			{#if type === 'udp' || type === 'dns'}
				<div class="space-y-1.5">
					<Label for="ck-probe">{type === 'udp' ? 'Payload to send' : 'Record type'}</Label>
					<Input id="ck-probe" bind:value={probe} placeholder={type === 'dns' ? 'A' : ''} />
				</div>
				<div class="space-y-1.5">
					<Label for="ck-expect">Expected answer</Label><Input id="ck-expect" bind:value={expect} />
				</div>
			{/if}
			{#if error}<p role="alert" class="text-md text-bad-text">{error}</p>{/if}
		</div>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			<Button variant="outline" onclick={() => (open = false)}>Cancel</Button>
			<Button onclick={save} disabled={busy || !name.trim() || !target.trim()}>Add check</Button>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>
