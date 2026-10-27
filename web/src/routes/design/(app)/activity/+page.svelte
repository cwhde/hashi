<script lang="ts">
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import { Input } from '$lib/components/ui/input';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Table from '$lib/components/ui/table';
	import { audit, jobs } from '$lib/design/mock';
	import { Search, Pause } from '@lucide/svelte';

	const log = [
		[
			'13:42:07.114',
			'info',
			'jobs',
			'job finished kind=monitor-checks duration_ms=1204 summary="23 endpoints"'
		],
		['13:42:06.901', 'info', 'ingest', 'access log read host=traefik-fra lines=18204 bytes=4.1 MB'],
		[
			'13:41:59.030',
			'warn',
			'blocklist',
			'fetch failed source="FireHOL Level 1" status=503, kept last good (3 d)'
		],
		['13:41:12.552', 'info', 'auth', 'reauth ok session=•••• credential="MacBook"'],
		[
			'13:40:52.880',
			'info',
			'edge',
			'deny subject=192.0.2.18 resource=vault.example.com reason=waf:942100'
		]
	];
	const lvl = {
		info: 'text-muted-foreground',
		warn: 'text-warn-text',
		error: 'text-bad-text'
	} as Record<string, string>;
</script>

<PageHeader
	title="Activity"
	description="Who changed what, what ran in the background, and the live log. Secrets never appear here."
/>
<Tabs.Root value="audit">
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
						placeholder="Filter audit entries"
						class="pl-8"
						aria-label="Filter audit entries"
					/>
				</div>
			</div>
			<Table.Root
				><Table.Header
					><Table.Row
						><Table.Head class="w-20 pl-4">Time</Table.Head><Table.Head class="w-24"
							>Actor</Table.Head
						><Table.Head>What</Table.Head><Table.Head class="hidden md:table-cell">Area</Table.Head
						><Table.Head class="hidden lg:table-cell">Result</Table.Head></Table.Row
					></Table.Header
				>
				<Table.Body
					>{#each audit as a, i (i)}<Table.Row
							><Table.Cell class="tnum pl-4">{a.t}</Table.Cell><Table.Cell>{a.who}</Table.Cell
							><Table.Cell class="font-medium">{a.what}</Table.Cell><Table.Cell
								class="hidden text-muted-foreground md:table-cell">{a.sub}</Table.Cell
							><Table.Cell class="hidden text-muted-foreground lg:table-cell">{a.result}</Table.Cell
							></Table.Row
						>{/each}</Table.Body
				>
			</Table.Root>
		</div>
	</Tabs.Content>
	<Tabs.Content value="jobs">
		<div class="rounded-2xl border bg-card">
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
								>{j.last}</Table.Cell
							><Table.Cell class="text-muted-foreground">{j.next}</Table.Cell><Table.Cell
								><div class="flex items-center gap-2">
									<Tag tone={j.status === 'succeeded' ? 'ok' : 'warn'}>{j.status}</Tag><span
										class="truncate text-muted-foreground">{j.summary}</span
									>
								</div></Table.Cell
							></Table.Row
						>{/each}</Table.Body
				>
			</Table.Root>
		</div>
	</Tabs.Content>
	<Tabs.Content value="log">
		<Panel title="Live log" description="Streaming · all subsystems · info and above" flush>
			{#snippet actions()}<button
					class="flex items-center gap-1 text-[13px] text-muted-foreground hover:text-foreground"
					><Pause class="size-4" />Pause</button
				>{/snippet}
			<ol class="overflow-x-auto p-3 font-mono text-[12.5px] leading-6">
				{#each log as [t, l, sub, msg], i (i)}<li class="whitespace-nowrap">
						<span class="text-muted-foreground">{t}</span>
						<span class={lvl[l ?? 'info']}>{(l ?? '').padEnd(5)}</span>
						<span class="text-primary">{sub}</span>
						{msg}
					</li>{/each}
			</ol>
		</Panel>
	</Tabs.Content>
</Tabs.Root>
