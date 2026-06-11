<script lang="ts">
	import { api, ApiRequestError } from '$lib/api/client';
	import { performPasskeyReauthentication } from '$lib/auth/reauth';
	import type { AuditEvent, BackgroundJob, SyncPlanPreview, SyncRun } from '$lib/api/types';
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import PanelSection from '$lib/components/layout/PanelSection.svelte';
	import { Button } from '$lib/components/ui/button';
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
	let runs = $state<SyncRun[]>([]);
	let jobs = $state<BackgroundJob[]>([]);
	let plan = $state<SyncPlanPreview | null>(null);
	let loading = $state(true);
	let syncing = $state(false);
	let error = $state<string | null>(null);
	let message = $state<string | null>(null);

	async function load() {
		loading = true;
		error = null;
		try {
			const [audit, syncRuns, backgroundJobs] = await Promise.all([
				api.getAuditEvents(),
				api.listSyncRuns(),
				api.listBackgroundJobs()
			]);
			events = audit;
			runs = syncRuns;
			jobs = backgroundJobs;
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to load activity';
		} finally {
			loading = false;
		}
	}

	async function planSync() {
		syncing = true;
		error = null;
		message = null;
		try {
			plan = await api.planGlobalSync();
			message = plan.requiresConfirmation
				? 'Plan ready — destructive changes require confirmation before apply.'
				: 'Plan ready — safe to apply.';
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Sync plan failed';
		} finally {
			syncing = false;
		}
	}

	async function applySync(confirmDestructive: boolean) {
		if (!plan) {
			error = 'Create and review a sync plan before applying changes.';
			return;
		}
		const approvedPlanId = plan.planId;
		syncing = true;
		error = null;
		message = null;
		try {
			const result = await api.applyGlobalSync(approvedPlanId, confirmDestructive);
			message = result.succeeded
				? `Apply completed (${result.status}).`
				: result.error ?? `Apply failed (${result.status}).`;
			plan = null;
			await load();
		} catch (e) {
			if (e instanceof ApiRequestError && e.code === 'reauth_required') {
				try {
					const ok = await performPasskeyReauthentication();
					if (ok) {
						await applySync(confirmDestructive);
						return;
					}
					error = 'Passkey reauthentication failed.';
				} catch (reauthError) {
					error =
						reauthError instanceof ApiRequestError
							? reauthError.message
							: 'Passkey reauthentication was cancelled.';
				}
			} else {
				error = e instanceof ApiRequestError ? e.message : 'Sync apply failed';
			}
		} finally {
			syncing = false;
		}
	}

	async function reconcileSync() {
		syncing = true;
		error = null;
		message = null;
		try {
			const result = await api.reconcileGlobalSync();
			message = result.succeeded
				? `Reconcile completed (${result.subsystemsReconciled.join(', ') || 'none'}).`
				: 'Reconcile finished with errors.';
			await load();
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Reconcile failed';
		} finally {
			syncing = false;
		}
	}

	$effect(() => {
		void load();
	});

	const pendingRuns = $derived(
		runs.filter((r) => r.status === 'awaiting_confirmation' || r.status === 'planning')
	);
</script>

<AdminSectionPage
	title="Activity"
	description="Audit log, sync runs, and global plan/apply controls."
	icon={Activity}
>
	<div class="space-y-4">
		<PanelSection title="Global sync" description="Plan, apply, and reconcile all subsystems.">
			<div class="flex flex-wrap items-center gap-2">
				<Button variant="outline" onclick={() => planSync()} disabled={syncing || loading}>
					Plan
				</Button>
				<Button onclick={() => applySync(false)} disabled={syncing || loading || !plan}>
					Apply safe changes
				</Button>
				<Button
					variant="destructive"
					onclick={() => applySync(true)}
					disabled={syncing || loading || !plan?.requiresConfirmation}
				>
					Apply with destructive confirm
				</Button>
				<Button variant="secondary" onclick={() => reconcileSync()} disabled={syncing || loading}>
					Reconcile
				</Button>
			</div>
			{#if message}
				<p class="mt-2 text-sm text-muted-foreground">{message}</p>
			{/if}
			{#if error}
				<p class="mt-2 text-sm text-destructive">{error}</p>
			{/if}
			{#if plan}
				<div class="mt-3 space-y-2 rounded-md border border-border bg-hashi-bg-dark/50 p-3">
					<p class="text-xs text-muted-foreground">
						Plan {plan.planId.slice(0, 8)} · risk {plan.riskLevel}
						{#if plan.requiresConfirmation}
							· confirmation required
						{/if}
					</p>
					{#if plan.previewMarkdown}
						<pre class="max-h-48 overflow-auto whitespace-pre-wrap font-mono text-[11px]">{plan.previewMarkdown}</pre>
					{/if}
				</div>
			{/if}
		</PanelSection>

		<PanelSection title="Sync runs" description="Recent plan/apply/reconcile history.">
			{#if loading}
				<p class="text-sm text-muted-foreground">Loading…</p>
			{:else if runs.length === 0}
				<p class="text-sm text-muted-foreground">No sync runs yet.</p>
			{:else}
				<div class="overflow-hidden rounded-md border border-border">
					<Table>
						<TableHeader>
							<TableRow>
								<TableHead>Started</TableHead>
								<TableHead>Subsystem</TableHead>
								<TableHead>Status</TableHead>
								<TableHead>Risk</TableHead>
								<TableHead>Changes</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{#each runs as run (run.id)}
								<TableRow>
									<TableCell class="whitespace-nowrap text-xs">
										{new Date(run.startedAtUtc).toLocaleString()}
									</TableCell>
									<TableCell>{run.subsystem}</TableCell>
									<TableCell>{run.status}</TableCell>
									<TableCell>{run.riskLevel ?? '—'}</TableCell>
									<TableCell>{run.diffs.length}</TableCell>
								</TableRow>
							{/each}
						</TableBody>
					</Table>
				</div>
				{#if pendingRuns.length > 0}
					<p class="mt-2 text-xs text-amber-400">
						{pendingRuns.length} run(s) awaiting confirmation or still planning.
					</p>
				{/if}
			{/if}
		</PanelSection>

		<PanelSection title="Background jobs" description="Last run, next run, duration, and errors (spec rule 22).">
			{#if loading}
				<p class="text-sm text-muted-foreground">Loading…</p>
			{:else if jobs.length === 0}
				<p class="text-sm text-muted-foreground">No background jobs registered yet.</p>
			{:else}
				<div class="overflow-hidden rounded-md border border-border">
					<Table>
						<TableHeader>
							<TableRow>
								<TableHead>Job</TableHead>
								<TableHead>Status</TableHead>
								<TableHead>Last run</TableHead>
								<TableHead>Next run</TableHead>
								<TableHead>Duration</TableHead>
								<TableHead>Summary</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{#each jobs as job (job.jobKey)}
								<TableRow>
									<TableCell>{job.displayName}</TableCell>
									<TableCell>{job.status}</TableCell>
									<TableCell class="text-xs whitespace-nowrap">
										{job.lastCompletedAtUtc
											? new Date(job.lastCompletedAtUtc).toLocaleString()
											: '—'}
									</TableCell>
									<TableCell class="text-xs whitespace-nowrap">
										{job.nextRunAtUtc ? new Date(job.nextRunAtUtc).toLocaleString() : '—'}
									</TableCell>
									<TableCell class="text-xs tabular-nums">
										{job.lastDurationMs != null ? `${job.lastDurationMs} ms` : '—'}
									</TableCell>
									<TableCell class="max-w-[14rem] truncate text-xs text-muted-foreground">
										{job.lastError ?? job.lastDiffSummary ?? '—'}
									</TableCell>
								</TableRow>
							{/each}
						</TableBody>
					</Table>
				</div>
			{/if}
		</PanelSection>

		<PanelSection title="Audit log" description="Privileged actions and outcomes.">
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
		</PanelSection>
	</div>
</AdminSectionPage>
