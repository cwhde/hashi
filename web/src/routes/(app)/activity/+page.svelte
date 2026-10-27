<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Table from '$lib/components/ui/table';
	import { Search, Pause, Play } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { dateTime } from '$lib/format';

	let tab = $state('audit');

	// Audit log: newest first, cursor paging (01-platform.md §7).
	let events = $state<Schemas['AuditEvent'][] | null>(null);
	let cursor = $state<string | undefined>();
	let auditError = $state('');
	let filter = $state('');
	const shown = $derived(
		(events ?? []).filter((e) =>
			`${e.action} ${e.actor} ${e.target} ${e.ip} ${e.outcome}`
				.toLowerCase()
				.includes(filter.toLowerCase())
		)
	);
	async function loadAudit(more = false) {
		const res = await api.GET('/api/activity/audit', {
			params: { query: { limit: 100, cursor: more ? cursor : undefined } }
		});
		if (res.error || !res.data) {
			auditError = problemText(res.error);
			return;
		}
		events = more ? [...(events ?? []), ...res.data.events] : res.data.events;
		cursor = res.data.next_cursor;
	}

	// Jobs (invariant 15).
	let jobs = $state<Schemas['Job'][] | null>(null);
	let jobsError = $state('');
	async function loadJobs() {
		const res = await api.GET('/api/activity/jobs');
		if (res.error || !res.data) jobsError = problemText(res.error);
		else jobs = res.data.jobs;
	}

	// Live log: polls the in-memory ring of already-redacted lines.
	type Line = {
		seq: number;
		time: string;
		level: string;
		component: string;
		msg: string;
		rest: string;
	};
	let lines = $state<Line[]>([]);
	let lastSeq = 0;
	let paused = $state(false);
	let logError = $state('');
	const known = new Set(['time', 'level', 'msg', 'component']);
	async function pollLog() {
		if (paused) return;
		const res = await api.GET('/api/activity/logs', { params: { query: { after: lastSeq } } });
		if (res.error || !res.data) {
			logError = problemText(res.error);
			return;
		}
		logError = '';
		lastSeq = res.data.last_seq;
		const fresh = res.data.lines.map(({ seq, line }) => ({
			seq,
			time: new Date(String(line.time)).toLocaleTimeString(undefined, { hour12: false }),
			level: String(line.level ?? 'INFO').toLowerCase(),
			component: String(line.component ?? ''),
			msg: String(line.msg ?? ''),
			rest: Object.entries(line)
				.filter(([k]) => !known.has(k))
				.map(([k, v]) => `${k}=${typeof v === 'string' ? v : JSON.stringify(v)}`)
				.join(' ')
		}));
		lines = [...fresh.reverse(), ...lines].slice(0, 500);
	}
	const lvl: Record<string, string> = {
		info: 'text-muted-foreground',
		debug: 'text-muted-foreground',
		warn: 'text-warn-text',
		error: 'text-bad-text'
	};

	onMount(() => {
		loadAudit();
		loadJobs();
		pollLog();
		const t = setInterval(() => {
			if (tab === 'log') pollLog();
			if (tab === 'jobs') loadJobs();
		}, 2000);
		return () => clearInterval(t);
	});

	const outcomeTone = { success: 'ok', failure: 'bad', denied: 'warn' } as const;
	const jobTone = { succeeded: 'ok', failed: 'bad', interrupted: 'warn', running: 'info' } as const;
</script>

<svelte:head><title>Activity · Hashi</title></svelte:head>
<PageHeader
	title="Activity"
	description="Who changed what, what ran in the background, and the live log. Secrets never appear here."
/>
{#snippet error(msg: string)}
	<p role="alert" class="p-4 text-[13px] text-bad-text">{msg}</p>
{/snippet}
{#snippet loading()}
	<div class="space-y-2 p-4">
		{#each Array(6).keys() as i (i)}<Skeleton class="h-9" />{/each}
	</div>
{/snippet}

<Tabs.Root bind:value={tab}>
	<Tabs.List class="mb-4"
		><Tabs.Trigger value="audit">Audit log</Tabs.Trigger><Tabs.Trigger value="jobs"
			>Jobs</Tabs.Trigger
		><Tabs.Trigger value="log">Live log</Tabs.Trigger></Tabs.List
	>
	<Tabs.Content value="audit">
		<div class="rounded-2xl border bg-card">
			<div class="border-b px-3 py-2.5">
				<div class="relative max-w-72">
					<Search
						class="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
					/><Input
						bind:value={filter}
						placeholder="Filter audit entries"
						class="pl-8"
						aria-label="Filter audit entries"
					/>
				</div>
			</div>
			{#if auditError}{@render error(auditError)}
			{:else if !events}{@render loading()}
			{:else if events.length === 0}
				<p class="p-4 text-[13px] text-muted-foreground">
					Nothing recorded yet. Sign-ins, setup steps and every change appear here.
				</p>
			{:else}
				<Table.Root
					><Table.Header
						><Table.Row
							><Table.Head class="w-28 pl-4">Time</Table.Head><Table.Head>What</Table.Head
							><Table.Head class="hidden md:table-cell">Actor</Table.Head><Table.Head
								class="hidden lg:table-cell">Target</Table.Head
							><Table.Head class="hidden md:table-cell">IP</Table.Head><Table.Head
								>Result</Table.Head
							></Table.Row
						></Table.Header
					>
					<Table.Body
						>{#each shown as e (e.id)}<Table.Row
								><Table.Cell class="tnum pl-4">{dateTime(e.at)}</Table.Cell><Table.Cell
									class="font-medium">{e.action}</Table.Cell
								><Table.Cell class="hidden md:table-cell"><Mono>{e.actor}</Mono></Table.Cell
								><Table.Cell class="hidden text-muted-foreground lg:table-cell"
									>{e.target}</Table.Cell
								><Table.Cell class="hidden md:table-cell"><Mono>{e.ip}</Mono></Table.Cell
								><Table.Cell><Tag tone={outcomeTone[e.outcome]}>{e.outcome}</Tag></Table.Cell
								></Table.Row
							>{/each}</Table.Body
					>
				</Table.Root>
				{#if cursor}
					<div class="border-t p-3 text-center">
						<Button variant="ghost" size="sm" onclick={() => loadAudit(true)}
							>Load older entries</Button
						>
					</div>
				{/if}
			{/if}
		</div>
	</Tabs.Content>
	<Tabs.Content value="jobs">
		<div class="rounded-2xl border bg-card">
			{#if jobsError}{@render error(jobsError)}
			{:else if !jobs}{@render loading()}
			{:else}
				<Table.Root
					><Table.Header
						><Table.Row
							><Table.Head class="pl-4">Job</Table.Head><Table.Head>Last run</Table.Head><Table.Head
								>Next</Table.Head
							><Table.Head>Result</Table.Head></Table.Row
						></Table.Header
					>
					<Table.Body
						>{#each jobs as j (j.kind)}<Table.Row
								><Table.Cell class="pl-4 font-medium">{j.kind}</Table.Cell><Table.Cell class="tnum"
									>{j.last_run
										? dateTime(j.last_run.started_at)
										: 'never'}{#if j.last_run?.duration_ms != null}<span
											class="text-muted-foreground"
										>
											· {j.last_run.duration_ms} ms</span
										>{/if}</Table.Cell
								><Table.Cell class="tnum text-muted-foreground"
									>{j.enabled ? dateTime(j.next_run_at) : 'off'}</Table.Cell
								><Table.Cell
									>{#if j.last_run}<div class="flex items-center gap-2">
											<Tag tone={jobTone[j.last_run.status]}>{j.last_run.status}</Tag><span
												class="truncate text-muted-foreground"
												>{j.last_run.error || j.last_run.summary}</span
											>
										</div>{:else}<span class="text-muted-foreground">waiting for first run</span
										>{/if}</Table.Cell
								></Table.Row
							>{/each}</Table.Body
					>
				</Table.Root>
			{/if}
		</div>
	</Tabs.Content>
	<Tabs.Content value="log">
		<Panel
			title="Live log"
			description={paused ? 'Paused' : 'Updating every 2 seconds · newest first'}
			flush
		>
			{#snippet actions()}<Button variant="ghost" size="sm" onclick={() => (paused = !paused)}
					>{#if paused}<Play />Resume{:else}<Pause />Pause{/if}</Button
				>{/snippet}
			{#if logError}{@render error(logError)}{/if}
			{#if lines.length === 0 && !logError}
				<p class="p-4 text-[13px] text-muted-foreground">No log lines since Hashi started.</p>
			{:else}
				<ol class="max-h-[60vh] overflow-auto p-3 font-mono text-[12.5px] leading-6">
					{#each lines as l (l.seq)}<li class="whitespace-nowrap">
							<span class="text-muted-foreground">{l.time}</span>
							<span class={lvl[l.level] ?? ''}>{l.level.padEnd(5)}</span>
							{#if l.component}<span class="text-primary">{l.component}</span>{/if}
							{l.msg} <span class="text-muted-foreground">{l.rest}</span>
						</li>{/each}
				</ol>
			{/if}
		</Panel>
	</Tabs.Content>
</Tabs.Root>
