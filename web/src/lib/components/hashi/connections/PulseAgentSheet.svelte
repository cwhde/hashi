<script lang="ts">
	// Add a Pulse agent and show its install once, with the token (08 §1, §3). A new token for an existing agent opens
	// the same install view (`shown`).
	import Mono from '#lib/components/hashi/Mono.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import * as Sheet from '#lib/components/ui/sheet/index.ts';
	import * as Tabs from '#lib/components/ui/tabs/index.ts';
	import { Copy } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';

	type Shown = { agent: Schemas['PulseAgent']; token: string };
	let {
		open = $bindable(false),
		shown = $bindable(null),
		onsaved
	}: { open?: boolean; shown?: Shown | null; onsaved: () => void } = $props();

	let name = $state('');
	let error = $state('');
	let busy = $state(false);
	$effect(() => {
		if (open && !shown) {
			name = '';
			error = '';
		}
	});
	async function create() {
		error = '';
		busy = true;
		try {
			const r = await api.POST('/api/pulse/agents', { body: { name: name.trim() } });
			if (!r.data) throw r.error;
			shown = r.data;
			onsaved();
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = false;
		}
	}

	let install = $state<Schemas['PulseInstall'] | null>(null);
	$effect(() => {
		const id = shown?.agent.id;
		install = null;
		if (!id) return;
		api
			.GET('/api/pulse/agents/{id}/install', { params: { path: { id } } })
			.then((r) => (install = r.data ?? null));
	});
	const fill = (t?: string) => (t && shown ? t.replaceAll('<token>', shown.token) : '');
	const linux = $derived(fill(install?.linux));
	const compose = $derived(fill(install?.compose));
	const copy = (t: string) => void navigator.clipboard?.writeText(t);
	function close() {
		open = false;
		shown = null;
	}
</script>

<Sheet.Root bind:open onOpenChange={(o) => !o && (shown = null)}>
	<Sheet.Content class="w-full sm:max-w-xl">
		<Sheet.Header>
			<Sheet.Title>{shown ? `Install ${shown.agent.name}` : 'Add a Pulse agent'}</Sheet.Title>
			<Sheet.Description>The token is shown once.</Sheet.Description>
		</Sheet.Header>
		<div class="space-y-4 overflow-y-auto px-4">
			{#if !shown}
				<div class="space-y-1.5">
					<Label for="agent-name">Name</Label><Input id="agent-name" bind:value={name} />
				</div>
				{#if error}<p role="alert" class="text-md text-bad-text">{error}</p>{/if}
			{:else}
				<Tabs.Root value="linux">
					<Tabs.List>
						<Tabs.Trigger value="linux">Linux</Tabs.Trigger>
						<Tabs.Trigger value="docker">Docker</Tabs.Trigger>
					</Tabs.List>
					<Tabs.Content value="linux">
						<div class="relative mt-2 rounded-xl border bg-surface-2/60 p-3 pr-10">
							<Mono class="break-all">{linux}</Mono>
							<Button
								variant="ghost"
								size="icon-sm"
								class="absolute top-2 right-2"
								aria-label="Copy command"
								onclick={() => copy(linux)}><Copy /></Button
							>
						</div>
					</Tabs.Content>
					<Tabs.Content value="docker">
						<div class="relative mt-2 rounded-xl border bg-surface-2/60 p-3 pr-10">
							<pre class="overflow-x-auto font-mono text-sm">{compose}</pre>
							<Button
								variant="ghost"
								size="icon-sm"
								class="absolute top-2 right-2"
								aria-label="Copy snippet"
								onclick={() => copy(compose)}><Copy /></Button
							>
						</div>
					</Tabs.Content>
				</Tabs.Root>
			{/if}
		</div>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			{#if shown}
				<Button onclick={close}>Done</Button>
			{:else}
				<Button variant="outline" onclick={close}>Cancel</Button>
				<Button onclick={create} disabled={busy || !name.trim()}>Create agent</Button>
			{/if}
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>
