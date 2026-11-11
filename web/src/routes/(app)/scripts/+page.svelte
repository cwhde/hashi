<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import PlanReview from '$lib/components/hashi/PlanReview.svelte';
	import ShellEditor from '$lib/components/hashi/ShellEditor.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { Switch } from '$lib/components/ui/switch';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { CircleAlert, Fingerprint, Play, Plus, Trash2, KeyRound, X } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { withReauth } from '$lib/auth/reauth';
	import { reviewItems, triggerText, outcomeText } from '$lib/sync';
	import { cn } from '$lib/utils';

	type Script = Schemas['Script'];
	type Run = Schemas['ScriptRun'];
	type Host = Schemas['FirewallHost'];
	type Env = { name: string; value: string; secret: boolean; stored: boolean };
	type Form = {
		name: string;
		description: string;
		body: string;
		enabled: boolean;
		schedule: string;
		timeoutMinutes: number;
		allHosts: boolean;
		hosts: string[];
		env: Env[];
	};

	const blank = (): Form => ({
		name: '',
		description: '',
		body: '#!/bin/sh\nset -eu\n\n',
		enabled: true,
		schedule: '',
		timeoutMinutes: 10,
		allHosts: true,
		hosts: [],
		env: []
	});

	let scripts = $state<Script[] | null>(null);
	let hosts = $state<Host[]>([]);
	let error = $state('');
	let selected = $state<string | null>(null); // a script id, 'new', or null
	let form = $state<Form>(blank());
	let saving = $state(false);
	let message = $state('');
	let formError = $state('');
	let runs = $state<Run[]>([]);
	let openRun = $state<Run | null>(null);
	let plans = $state<Schemas['SyncPlan'][]>([]);
	let planBusy = $state(false);
	let preview = $state<Schemas['SchedulePreview'] | null>(null);
	let previewError = $state('');
	let confirmDelete = $state(false);

	const current = $derived(scripts?.find((s) => s.id === selected) ?? null);
	// Hosts Hashi reaches over SSH: only those can hold scripts.
	const reachable = $derived(hosts.filter((h) => h.host !== '' && h.host_key_fingerprint !== ''));
	const pending = $derived(current?.targets.filter((t) => t.pending) ?? []);

	async function load() {
		const [s, h] = await Promise.all([api.GET('/api/scripts'), api.GET('/api/firewall/hosts')]);
		if (s.error || !s.data) {
			error = problemText(s.error);
			return;
		}
		scripts = s.data.scripts;
		if (h.data) hosts = h.data.hosts;
		if (selected === null && scripts[0]) pick(scripts[0].id);
	}
	onMount(load);

	function formOf(s: Script): Form {
		return {
			name: s.name,
			description: s.description,
			body: s.body,
			enabled: s.enabled,
			schedule: s.schedule,
			timeoutMinutes: Math.max(1, Math.round(s.timeout_seconds / 60)),
			allHosts: s.all_hosts,
			hosts: [...s.hosts],
			env: s.env.map((e) => ({
				name: e.name,
				value: e.value ?? '',
				secret: e.secret,
				stored: e.secret
			}))
		};
	}

	async function pick(id: string | null) {
		selected = id;
		message = formError = '';
		openRun = null;
		plans = [];
		const s = scripts?.find((x) => x.id === id);
		form = s ? formOf(s) : blank();
		runs = [];
		if (s) await loadRuns(s.id);
	}

	async function loadRuns(id: string) {
		const r = await api.GET('/api/scripts/{id}/runs', { params: { path: { id } } });
		if (r.data && selected === id) runs = r.data.runs;
	}

	// The schedule preview: the next firing times, so the admin sees what the cron line means.
	let previewTimer: ReturnType<typeof setTimeout> | undefined;
	$effect(() => {
		const sched = form.schedule.trim();
		clearTimeout(previewTimer);
		preview = null;
		previewError = '';
		if (!sched) return;
		previewTimer = setTimeout(async () => {
			const r = await api.GET('/api/scripts/schedule-preview', {
				params: { query: { schedule: sched } }
			});
			if (form.schedule.trim() !== sched) return;
			if (r.error) previewError = problemText(r.error);
			else preview = r.data ?? null;
		}, 300);
	});

	const fmt = (iso: string) =>
		new Date(iso).toLocaleString(undefined, {
			weekday: 'short',
			day: 'numeric',
			month: 'short',
			hour: '2-digit',
			minute: '2-digit'
		});

	function body(): Schemas['ScriptInput'] {
		return {
			name: form.name.trim(),
			description: form.description,
			body: form.body,
			enabled: form.enabled,
			schedule: form.schedule.trim(),
			timeout_seconds: Math.round(form.timeoutMinutes * 60),
			all_hosts: form.allHosts,
			hosts: form.allHosts ? [] : form.hosts,
			// A stored secret left empty keeps its value.
			env: form.env.map((e) =>
				e.secret && e.stored && e.value === ''
					? { name: e.name, secret: true }
					: { name: e.name, value: e.value, secret: e.secret }
			)
		};
	}

	async function save() {
		saving = true;
		formError = message = '';
		try {
			const saved =
				selected === 'new' || !current
					? await withReauth(() => api.POST('/api/scripts', { body: body() }))
					: await withReauth(() =>
							api.PUT('/api/scripts/{id}', { params: { path: { id: current.id } }, body: body() })
						);
			await load();
			await pick(saved.id);
			message = saved.targets.some((t) => t.pending)
				? 'Saved. The change waits for you below before it reaches the hosts.'
				: 'Saved.';
			if (saved.targets.some((t) => t.pending)) await plan();
		} catch (e) {
			formError = problemText(e);
		} finally {
			saving = false;
		}
	}

	async function plan() {
		if (!current) return;
		planBusy = true;
		const r = await api.POST('/api/scripts/{id}/plan', { params: { path: { id: current.id } } });
		planBusy = false;
		if (r.error) formError = problemText(r.error);
		else plans = r.data.plans;
	}

	async function apply() {
		if (!current) return;
		planBusy = true;
		formError = '';
		try {
			const id = current.id;
			const res = await withReauth(() =>
				api.POST('/api/scripts/{id}/apply', { params: { path: { id } } })
			);
			plans = res.plans;
			const failed = res.plans.filter((p) => p.status === 'failed');
			message = failed.length ? '' : 'Applied. The hosts hold the saved script now.';
			if (failed.length) formError = failed.map((p) => `${p.target_name}: ${p.error}`).join(' ');
			await load();
		} catch (e) {
			formError = problemText(e);
		} finally {
			planBusy = false;
		}
	}

	async function run(hostId: string) {
		if (!current) return;
		formError = '';
		try {
			const id = current.id;
			const started = await withReauth(() =>
				api.POST('/api/scripts/{id}/run', { params: { path: { id } }, body: { host_id: hostId } })
			);
			await loadRuns(id);
			await follow(started.run_id);
		} catch (e) {
			formError = problemText(e);
		}
	}

	// Polls a run until it finishes, showing its output as soon as it is there.
	async function follow(runId: string) {
		for (;;) {
			const r = await api.GET('/api/script-runs/{runId}', { params: { path: { runId } } });
			if (!r.data) return;
			openRun = r.data;
			if (r.data.status !== 'running') break;
			await new Promise((res) => setTimeout(res, 1000));
		}
		if (current) await loadRuns(current.id);
		await load();
	}

	async function show(r: Run) {
		if (openRun?.id === r.id) {
			openRun = null;
			return;
		}
		const res = await api.GET('/api/script-runs/{runId}', { params: { path: { runId: r.id } } });
		if (res.data) openRun = res.data;
	}

	async function remove() {
		if (!current) return;
		try {
			const id = current.id;
			await withReauth(() => api.DELETE('/api/scripts/{id}', { params: { path: { id } } }));
			confirmDelete = false;
			selected = null;
			await load();
			message = 'Deleted. Each host it ran on has a waiting change that removes its files.';
		} catch (e) {
			formError = problemText(e);
		}
	}

	const tone = {
		succeeded: 'ok',
		failed: 'bad',
		timed_out: 'warn',
		error: 'bad',
		running: 'info'
	} as const;
	const statusText = {
		succeeded: 'succeeded',
		failed: 'failed',
		timed_out: 'timed out',
		error: 'not run',
		running: 'running'
	} as const;
	const duration = (r: Run) =>
		r.finished_at
			? `${((new Date(r.finished_at).getTime() - new Date(r.started_at).getTime()) / 1000).toFixed(1)} s`
			: '';
</script>

<svelte:head><title>Scripts · Hashi</title></svelte:head>
<PageHeader
	title="Scripts"
	description="Shell scripts Hashi keeps on your firewall hosts and runs on a schedule or by hand. They run as root, so saving and running ask for your passkey."
>
	{#snippet actions()}<Button onclick={() => pick('new')}><Plus />New script</Button>{/snippet}
</PageHeader>

{#if error}
	<p class="flex items-center gap-2 text-[13px] text-bad-text">
		<CircleAlert class="size-3.5" />{error}
	</p>
{:else if !scripts}
	<Skeleton class="h-64 w-full rounded-2xl" />
{:else}
	<div class="grid gap-4 lg:grid-cols-[18rem_minmax(0,1fr)]">
		<nav class="h-fit rounded-2xl border bg-card p-1.5" aria-label="Scripts">
			{#each scripts as x (x.id)}
				<button
					onclick={() => pick(x.id)}
					aria-current={selected === x.id ? 'true' : undefined}
					class={cn(
						'w-full rounded-xl px-3 py-2.5 text-left',
						selected === x.id ? 'bg-surface-2' : 'hover:bg-surface-2/50'
					)}
				>
					<div class="flex items-center gap-2 text-[13.5px] font-medium">
						<span class="truncate">{x.name}</span>
						{#if !x.enabled}<Tag>off</Tag>{/if}
						{#if x.targets.some((t) => t.pending)}<Tag tone="warn">waiting</Tag>{/if}
					</div>
					<div class="mt-1 flex items-center gap-1" aria-label="Last runs">
						{#each [...x.last_runs].reverse() as r, i (i)}<span
								class={cn(
									'h-3 w-1.5 rounded-sm',
									r === 'succeeded' ? 'bg-ok/80' : r === 'running' ? 'bg-info/70' : 'bg-bad'
								)}
								title={statusText[r]}
							></span>{/each}
						<span class="ml-1.5 truncate text-[12px] text-muted-foreground"
							>{#if x.schedule}<Mono>{x.schedule}</Mono>{:else}by hand{/if}</span
						>
					</div>
				</button>
			{:else}
				<p class="px-3 py-2.5 text-[13px] text-muted-foreground">
					No scripts yet. Add one to run it on your firewall hosts.
				</p>
			{/each}
		</nav>

		{#if selected !== null}
			<div class="min-w-0 space-y-4">
				<Panel
					title={selected === 'new' ? 'New script' : form.name || 'Script'}
					description={current
						? current.targets.length
							? `Runs on ${current.targets.map((t) => t.name).join(', ')}`
							: 'Runs on no host'
						: 'Saving puts it on the hosts after you approve the change.'}
					flush
				>
					{#snippet actions()}
						{#if current && current.enabled && current.targets.length > 0}
							{#if current.targets.length === 1}
								<Button size="sm" variant="outline" onclick={() => run(current.targets[0]!.id)}
									><Play />Run now</Button
								>
							{:else}
								<DropdownMenu.Root>
									<DropdownMenu.Trigger>
										{#snippet child({ props })}<Button {...props} size="sm" variant="outline"
												><Play />Run now</Button
											>{/snippet}
									</DropdownMenu.Trigger>
									<DropdownMenu.Content align="end">
										{#each current.targets as t (t.id)}<DropdownMenu.Item onclick={() => run(t.id)}
												>On {t.name}</DropdownMenu.Item
											>{/each}
									</DropdownMenu.Content>
								</DropdownMenu.Root>
							{/if}
						{/if}
						<Button size="sm" disabled={saving} onclick={save}><Fingerprint />Save</Button>
					{/snippet}
					<div class="grid gap-4 border-b p-4 md:grid-cols-2">
						<div class="space-y-1.5">
							<Label for="sname">Name</Label><Input id="sname" bind:value={form.name} />
							{#if current}<p class="text-[12px] text-muted-foreground">
									File on the hosts: <Mono>{current.slug}.sh</Mono>
								</p>{/if}
						</div>
						<div class="space-y-1.5">
							<Label for="sdesc">Description</Label><Input
								id="sdesc"
								bind:value={form.description}
							/>
						</div>
					</div>
					<div class="border-b bg-surface-2/40">
						<ShellEditor bind:value={form.body} label="Script" />
					</div>
					<div class="grid gap-4 p-4 md:grid-cols-3">
						<div class="space-y-1.5">
							<Label for="ssched">Schedule (cron, host time)</Label><Input
								id="ssched"
								bind:value={form.schedule}
								placeholder="by hand only"
								class="font-mono"
							/>
							{#if previewError}<p class="text-[12px] text-bad-text">{previewError}</p>
							{:else if preview}<p class="text-[12px] text-muted-foreground">
									Next: {preview.next.slice(0, 3).map(fmt).join(', ')}
								</p>
							{:else if !form.schedule.trim()}<p class="text-[12px] text-muted-foreground">
									e.g. <Mono>30 3 * * 0</Mono> is Sundays at 03:30
								</p>{/if}
						</div>
						<div class="space-y-1.5">
							<Label for="stime">Timeout (minutes)</Label><Input
								id="stime"
								type="number"
								min="1"
								max="1440"
								bind:value={form.timeoutMinutes}
							/>
						</div>
						<div class="flex items-center gap-3 pt-6">
							<Switch id="sen" bind:checked={form.enabled} /><Label for="sen"
								>{form.enabled ? 'On' : 'Off: removed from the hosts'}</Label
							>
						</div>
					</div>
					<fieldset class="space-y-2 border-t px-4 py-3">
						<legend class="sr-only">Environment variables</legend>
						<div class="flex items-center justify-between">
							<span class="text-[13px] font-medium">Environment variables</span>
							<Button
								size="sm"
								variant="ghost"
								onclick={() => form.env.push({ name: '', value: '', secret: false, stored: false })}
								><Plus />Add variable</Button
							>
						</div>
						{#each form.env as e, i (i)}
							<div
								class="grid grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)_auto_auto] items-center gap-2"
							>
								<Input
									bind:value={e.name}
									placeholder="NAME"
									class="font-mono"
									aria-label="Variable name"
								/>
								<Input
									bind:value={e.value}
									type={e.secret ? 'password' : 'text'}
									placeholder={e.secret && e.stored ? 'stored, leave empty to keep' : 'value'}
									class="font-mono"
									aria-label="Value of {e.name || 'the variable'}"
								/>
								<label class="flex items-center gap-1.5 text-[12.5px]"
									><Checkbox bind:checked={e.secret} /><KeyRound class="size-3.5" />secret</label
								>
								<Button
									size="icon"
									variant="ghost"
									aria-label="Remove {e.name || 'variable'}"
									onclick={() => form.env.splice(i, 1)}><X /></Button
								>
							</div>
						{:else}
							<p class="text-[12.5px] text-muted-foreground">
								None. Secret values stay in Hashi's vault and are replaced in run output.
							</p>
						{/each}
					</fieldset>
					<fieldset class="border-t px-4 py-3">
						<legend class="sr-only">Target hosts</legend>
						<div class="flex flex-wrap items-center gap-x-4 gap-y-2 text-[13px]">
							<span class="text-muted-foreground">Runs on</span>
							<label class="flex items-center gap-2"
								><Checkbox bind:checked={form.allHosts} />every firewall host</label
							>
							{#if !form.allHosts}
								{#each reachable as h (h.id)}
									<label class="flex items-center gap-2"
										><Checkbox
											checked={form.hosts.includes(h.id)}
											onCheckedChange={(v) =>
												(form.hosts = v
													? [...form.hosts, h.id]
													: form.hosts.filter((x) => x !== h.id))}
										/>{h.name}</label
									>
								{:else}
									<span class="text-muted-foreground"
										>No firewall host is reachable over SSH yet.</span
									>
								{/each}
							{/if}
						</div>
						{#if !form.allHosts && form.hosts.length === 0}<p
								class="mt-1.5 text-[12px] text-warn-text"
							>
								No host picked: the script runs nowhere.
							</p>{/if}
					</fieldset>
					{#if formError || message || current}
						<div class="flex flex-wrap items-center gap-3 border-t px-4 py-3">
							{#if formError}<p role="alert" class="mr-auto text-[13px] text-bad-text">
									{formError}
								</p>
							{:else if message}<p role="status" class="mr-auto text-[13px] text-muted-foreground">
									{message}
								</p>
							{:else}<span class="mr-auto"></span>{/if}
							{#if current}<Button
									size="sm"
									variant="ghost"
									class="text-bad-text"
									onclick={() => (confirmDelete = true)}><Trash2 />Delete</Button
								>{/if}
						</div>
					{/if}
				</Panel>

				{#if current && (pending.length > 0 || plans.length > 0)}
					<section class="space-y-3" aria-label="Waiting changes">
						{#if plans.length === 0}
							<div class="flex flex-wrap items-center gap-3 rounded-2xl border bg-card px-4 py-3">
								<p class="mr-auto text-[13px]">
									A change waits for {pending.map((t) => t.name).join(', ')}.
								</p>
								<Button size="sm" variant="outline" disabled={planBusy} onclick={plan}
									>Show the change</Button
								>
							</div>
						{/if}
						{#each plans as p (p.id)}
							<PlanReview
								title="{p.target_name}: {p.title}"
								trigger={triggerText(p)}
								risk={p.risk}
								items={reviewItems(p)}
							>
								{#snippet badge()}
									{#if p.status === 'pending' || p.status === 'approved'}<Tag tone="warn"
											>waiting</Tag
										>{:else}<Tag tone={p.status === 'applied' ? 'ok' : 'bad'}>{outcomeText(p)}</Tag
										>{/if}
								{/snippet}
								{#snippet footer()}
									<p class="mr-auto text-[12.5px] text-muted-foreground">
										Applying puts every waiting script change for this host in place. It asks for
										your passkey.
									</p>
									{#if p.status === 'pending' || p.status === 'approved'}<Button
											size="sm"
											disabled={planBusy}
											onclick={apply}><Fingerprint />Approve and apply</Button
										>{/if}
								{/snippet}
							</PlanReview>
						{/each}
					</section>
				{/if}

				{#if current}
					<Panel title="Runs" flush>
						{#if openRun && !runs.some((r) => r.id === openRun?.id)}
							<p class="px-4 py-2.5 text-[13px] text-muted-foreground">Starting…</p>
						{/if}
						<ul class="divide-y text-[13px]">
							{#each runs as r (r.id)}
								<li>
									<button
										class="flex w-full items-center gap-3 px-4 py-2.5 text-left hover:bg-surface-2/40"
										aria-expanded={openRun?.id === r.id}
										onclick={() => show(r)}
									>
										<Tag tone={tone[r.status]}>{statusText[r.status]}</Tag>
										<span class="min-w-0 flex-1 truncate"
											>{fmt(r.started_at)} · {r.host_name}{#if r.exit_code !== undefined && r.exit_code !== 0}
												· exit {r.exit_code}{/if}{#if duration(r)}
												· {duration(r)}{/if}</span
										>
										<span class="hidden text-muted-foreground sm:inline"
											>{r.trigger === 'manual' ? 'by hand' : 'scheduled'}</span
										>
									</button>
									{#if openRun?.id === r.id}
										<div class="px-4 pb-3">
											{#if openRun.error}<p class="mb-2 text-[12.5px] text-bad-text">
													{openRun.error}
												</p>{/if}
											{#if openRun.output}<pre
													class="max-h-96 overflow-auto rounded-lg bg-surface-2/60 p-2.5 font-mono text-[12px] whitespace-pre-wrap"
													aria-label="Output">{openRun.output}</pre>
											{:else if !openRun.error}<p class="text-[12.5px] text-muted-foreground">
													{openRun.status === 'running' ? 'Running…' : 'No output.'}
												</p>{/if}
											{#if openRun.truncated}<p class="mt-1 text-[12px] text-muted-foreground">
													Output cut at 1 MB.
												</p>{/if}
										</div>
									{/if}
								</li>
							{:else}
								<li class="px-4 py-2.5 text-muted-foreground">No runs yet.</li>
							{/each}
						</ul>
					</Panel>
				{/if}
			</div>
		{/if}
	</div>
{/if}

<AlertDialog.Root bind:open={confirmDelete}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>Delete {current?.name}?</AlertDialog.Title>
			<AlertDialog.Description
				>Its runs go too. On each host it ran on, a waiting change removes the script, its variables
				and its timer once you apply it.</AlertDialog.Description
			>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>Cancel</AlertDialog.Cancel>
			<AlertDialog.Action onclick={remove}><Fingerprint />Delete</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
