<script lang="ts">
	import { api } from '$lib/api/client';
	import type { AuditEvent } from '$lib/api/types';
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import {
		Table,
		TableBody,
		TableCell,
		TableHead,
		TableHeader,
		TableRow
	} from '$lib/components/ui/table';
	import { Activity } from 'lucide-svelte';

	let events = $state<AuditEvent[]>([]);
	let loading = $state(true);

	$effect(() => {
		let cancelled = false;

		void (async () => {
			try {
				const data = await api.getAuditEvents();
				if (!cancelled) events = data;
			} catch {
				if (!cancelled) events = [];
			} finally {
				if (!cancelled) loading = false;
			}
		})();

		return () => {
			cancelled = true;
		};
	});
</script>

<AdminSectionPage
	title="Activity"
	description="Audit log of privileged actions and sync outcomes."
	icon={Activity}
>
	{#if loading}
		<p class="text-sm text-muted-foreground">Loading audit log…</p>
	{:else if events.length === 0}
		<p class="text-sm text-muted-foreground">No audit entries yet.</p>
	{:else}
		<div class="overflow-hidden rounded-md border border-border">
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>Time</TableHead>
						<TableHead>Category</TableHead>
						<TableHead>Action</TableHead>
						<TableHead>Subject</TableHead>
						<TableHead>Outcome</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{#each events as event (event.id)}
						<TableRow>
							<TableCell class="whitespace-nowrap text-xs">
								{new Date(event.createdAtUtc).toLocaleString()}
							</TableCell>
							<TableCell>{event.category}</TableCell>
							<TableCell>{event.action}</TableCell>
							<TableCell class="max-w-[10rem] truncate text-xs">
								{event.subjectType ?? '—'}{event.subjectId ? ` / ${event.subjectId}` : ''}
							</TableCell>
							<TableCell>{event.outcome}</TableCell>
						</TableRow>
					{/each}
				</TableBody>
			</Table>
		</div>
	{/if}
</AdminSectionPage>
