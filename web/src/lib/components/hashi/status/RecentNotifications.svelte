<script lang="ts">
	import { onMount } from 'svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Send, Settings } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { dateTime } from '$lib/format';

	// 07 §6: what was sent and its result. Where alerts go is set up under Settings → Notifications.
	let deliveries = $state<Schemas['NotificationDelivery'][]>([]);
	let target = $state<string | null>(null);
	let error = $state('');

	onMount(async () => {
		const [d, r, p] = await Promise.all([
			api.GET('/api/notifications/deliveries'),
			api.GET('/api/notifications/route'),
			api.GET('/api/notifications/providers')
		]);
		if (!d.data || !r.data || !p.data) {
			error = problemText(d.error ?? r.error ?? p.error);
			return;
		}
		deliveries = d.data.deliveries;
		target = p.data.providers.find((x) => x.id === r.data.provider_id)?.name ?? null;
	});
	const deliveryText = (d: Schemas['NotificationDelivery']) =>
		d.status === 'delivered'
			? 'delivered'
			: d.status === 'failed'
				? `failed: ${d.last_error}`
				: d.last_error
					? `retrying: ${d.last_error}`
					: 'waiting';
</script>

{#if error}<p role="alert" class="mb-3 text-[13px] text-bad-text">{error}</p>{/if}
<Panel
	title="Recent notifications"
	description={target
		? `Alerts go to ${target}.`
		: 'No alerts are sent until you choose a provider.'}
>
	{#snippet actions()}<Button variant="outline" size="sm" href="/settings?category=notifications"
			><Settings />Notification settings</Button
		>{/snippet}
	{#if deliveries.length === 0}
		<p class="text-[13px] text-muted-foreground">Nothing sent yet.</p>
	{/if}
	<ul class="space-y-2.5 text-[13px]">
		{#each deliveries as d (d.id)}
			<li class="flex flex-wrap items-center gap-x-3 gap-y-0.5">
				<Send class="size-4 text-muted-foreground" />
				<span class="min-w-40 flex-1">{d.subject}</span>
				<span class={d.status === 'delivered' ? 'text-muted-foreground' : 'text-bad-text'}
					>{d.provider} · {dateTime(d.created_at)} · {deliveryText(d)}</span
				>
			</li>
		{/each}
	</ul>
</Panel>
