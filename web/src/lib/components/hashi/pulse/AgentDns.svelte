<script lang="ts">
	import { onMount } from 'svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import * as Select from '$lib/components/ui/select';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { withReauth } from '$lib/auth/reauth';

	// 03 §9: stable internal names for Pulse agents, written through AdGuard.
	let v = $state<Schemas['AgentDNSSettings'] | null>(null);
	let error = $state('');
	let notice = $state('');
	let busy = $state(false);
	onMount(async () => {
		const r = await api.GET('/api/settings/internal-agent-dns');
		if (r.data) v = r.data;
		else error = problemText(r.error);
	});
	async function save() {
		error = notice = '';
		busy = true;
		try {
			v = await withReauth(() => api.PUT('/api/settings/internal-agent-dns', { body: v! }));
			notice = 'Saved.';
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = false;
		}
	}
	const modes = { selected: "The agent's selected internal address", public: 'Its public address' };
</script>

{#if v}
	<Panel
		title="Internal agent DNS"
		description="Gives each Pulse agent a name through AdGuard. DNS only, not reverse proxy: no service is created at these names."
	>
		<div class="space-y-3 text-[13px]">
			<label class="flex items-center justify-between gap-3">
				<span>Create names for agents</span><Switch
					bind:checked={v.enabled}
					aria-label="Create names for agents"
				/>
			</label>
			<div class="grid gap-3 sm:grid-cols-2">
				<div class="space-y-1.5">
					<Label for="ad-domain">Internal domain</Label><Input
						id="ad-domain"
						bind:value={v.domain}
					/>
				</div>
				<div class="space-y-1.5">
					<Label for="ad-mode">Answer with</Label>
					<Select.Root type="single" bind:value={v.ip_mode}>
						<Select.Trigger id="ad-mode" class="w-full">{modes[v.ip_mode]}</Select.Trigger>
						<Select.Content>
							<Select.Item value="selected">{modes.selected}</Select.Item>
							<Select.Item value="public">{modes.public}</Select.Item>
						</Select.Content>
					</Select.Root>
				</div>
			</div>
			<div class="flex items-center justify-end gap-3">
				{#if notice}<span role="status" class="text-ok-text">{notice}</span>{/if}
				<Button size="sm" onclick={save} disabled={busy}>Save</Button>
			</div>
		</div>
	</Panel>
{/if}
{#if error}<p role="alert" class="mt-3 text-[13px] text-bad-text">{error}</p>{/if}
