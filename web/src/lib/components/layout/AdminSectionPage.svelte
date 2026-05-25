<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { LucideIcon } from '$lib/icons';
	import PageHeader from '$lib/components/layout/PageHeader.svelte';
	import PanelSection from '$lib/components/layout/PanelSection.svelte';
	import ApiPendingBanner from '$lib/components/layout/ApiPendingBanner.svelte';

	let {
		title,
		description,
		icon,
		pendingFeature,
		pendingDetail,
		toolbar,
		children
	}: {
		title: string;
		description: string;
		icon: LucideIcon;
		pendingFeature?: string;
		pendingDetail?: string;
		toolbar?: Snippet;
		children?: Snippet;
	} = $props();
</script>

<section class="mx-auto max-w-7xl space-y-4">
	<PageHeader {title} {description} {icon}>
		{#snippet actions()}
			{#if toolbar}
				{@render toolbar()}
			{/if}
		{/snippet}
	</PageHeader>

	{#if pendingFeature}
		<ApiPendingBanner message={pendingFeature} detail={pendingDetail} />
	{/if}

	<PanelSection title="Operational view" description="Dense table and sync controls per spec.">
		{#if children}
			{@render children()}
		{:else}
			<p class="text-sm text-muted-foreground">
				Route shell ready — connect list, plan, and apply endpoints when backend ships.
			</p>
		{/if}
	</PanelSection>
</section>
