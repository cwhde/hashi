<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import type { TraefikRenderResponse } from '$lib/api/types';
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import PanelSection from '$lib/components/layout/PanelSection.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Network } from 'lucide-svelte';

	let render = $state<TraefikRenderResponse | null>(null);
	let loading = $state(false);
	let error = $state<string | null>(null);

	async function loadRender() {
		loading = true;
		error = null;
		try {
			render = await api.getTraefikRender();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to render Traefik config';
			render = null;
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		void loadRender();
	});
</script>

<AdminSectionPage
	title="Traefik"
	description="Connection health, config ownership, routers, and reconcile state."
	icon={Network}
>
	<div class="flex items-center gap-2">
		<Button variant="outline" onclick={() => loadRender()} disabled={loading}>
			{loading ? 'Rendering…' : 'Refresh render'}
		</Button>
		{#if render}
			<span class="text-xs text-muted-foreground">hash {render.contentHash}</span>
		{/if}
	</div>
	{#if error}
		<p class="text-sm text-destructive">{error}</p>
	{/if}

	{#if render}
		<PanelSection title="Static config" description="Generated traefik.yml preview.">
			<pre
				class="max-h-96 overflow-auto rounded-md border border-border bg-hashi-bg-dark p-3 font-mono text-[11px] text-hashi-foreground">{render.staticConfigYaml}</pre>
		</PanelSection>
		<PanelSection title="Dynamic HTTP" description="Generated dynamic configuration.">
			<pre
				class="max-h-96 overflow-auto rounded-md border border-border bg-hashi-bg-dark p-3 font-mono text-[11px] text-hashi-foreground">{render.dynamicHttpYaml}</pre>
		</PanelSection>
	{/if}
</AdminSectionPage>
