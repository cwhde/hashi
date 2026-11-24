<script lang="ts">
	import { goto } from '$app/navigation';
	import * as Command from '#lib/components/ui/command/index.ts';
	import Mono from '#lib/components/hashi/Mono.svelte';
	import { api, type Schemas } from '#lib/api/client.ts';
	import { localSearch, type LocalGroup } from '#lib/search/local.ts';

	// The top bar search (10b §2, bug 4). Pages, tabs and settings are found here at once; what Hashi stores comes from
	// /api/search a moment later. The list is ours, so the command filter is off and the order is the groups' own.
	let {
		open = $bindable(false),
		pages,
		live
	}: { open?: boolean; pages: { label: string; href: string }[]; live: boolean } = $props();

	let query = $state('');
	let server = $state<Schemas['SearchResults'] | null>(null);
	let asked = $state(''); // the query `server` answers
	let failed = $state(false);
	let seq = 0;

	const q = $derived(query.trim());
	const local = $derived<LocalGroup[]>(
		q
			? localSearch(q, pages)
			: [{ label: 'Pages', hits: pages.map((p) => ({ title: p.label, detail: '', href: p.href })) }]
	);
	const waiting = $derived(live && q !== '' && asked !== q && !failed);
	const groups = $derived(server && asked === q ? server.groups : []);
	const address = $derived(server && asked === q ? server.address : undefined);
	type Hit = { title: string; detail: string; href: string };
	type Section = { heading: string; hits: Hit[]; more: number };
	// Display order: the address, pages, then stored objects, then tabs and settings.
	const sections = $derived.by<Section[]>(() => {
		const out: Section[] = [];
		const ip = address;
		if (ip)
			out.push({
				heading: 'Address',
				hits: [
					{
						title: `Look up ${ip}`,
						detail: 'Sets, firewall and history',
						href: `/ip-sets/address?ip=${encodeURIComponent(ip)}`
					}
				],
				more: 0
			});
		if (!live && q)
			out.push({
				heading: 'Services',
				hits: [{ title: 'git.example.com', detail: 'Example data', href: '/design/services' }],
				more: 0
			});
		const fromLocal = (g: LocalGroup) => ({ heading: g.label, hits: g.hits, more: 0 });
		const rest: Section[] = [
			...local.filter((g) => g.label === 'Pages').map(fromLocal),
			...groups.map((g) => ({ heading: g.label, hits: g.results, more: g.more })),
			...local.filter((g) => g.label !== 'Pages').map(fromLocal)
		];
		// A group whose best result is named exactly what was typed, or starts with it, comes first.
		const best = (sec: Section) => {
			const t = sec.hits[0]?.title.toLowerCase() ?? '';
			const n = q.toLowerCase();
			return t === n ? 0 : t.startsWith(n) ? 1 : 2;
		};
		return [
			...out,
			...rest
				.map((sec, i) => ({ sec, i }))
				.sort((a, b) => best(a.sec) - best(b.sec) || a.i - b.i)
				.map((x) => x.sec)
		];
	});
	const nothing = $derived(q !== '' && !waiting && sections.length === 0);
	// Two results may share a title and a place (two records of one name), so the key also holds the position.
	const key = (sec: Section, i: number) => `${sec.heading}|${i}|${sec.hits[i]?.title}`;
	// Enter opens the top result: the selection moves to it whenever the results change, also when the server's arrive.
	let selected = $state('');
	$effect(() => {
		const top = sections[0];
		selected = top?.hits.length ? key(top, 0) : '';
	});

	// Ask the server once typing pauses; only the answer to the latest query is shown.
	$effect(() => {
		const now = q;
		failed = false;
		if (!live || !now) return;
		const mine = ++seq;
		const t = setTimeout(async () => {
			const r = await api.GET('/api/search', { params: { query: { q: now } } });
			if (mine !== seq) return;
			if (r.data) [server, asked] = [r.data, now];
			else failed = true;
		}, 150);
		return () => clearTimeout(t);
	});
	// A new search starts empty.
	$effect(() => {
		if (!open) query = '';
	});

	function go(href: string) {
		open = false;
		void goto(href);
	}
</script>

<Command.Dialog
	bind:open
	bind:value={selected}
	shouldFilter={false}
	class="sm:max-w-xl"
	label="Search Hashi"
	title="Search Hashi"
	description="Addresses, services, records, settings and pages"
>
	<Command.Input bind:value={query} placeholder="Address, service, domain, setting or page…" />
	<Command.List class="max-h-[min(28rem,60vh)]">
		{#each sections as sec (sec.heading)}
			<Command.Group heading={sec.heading}>
				{#each sec.hits as h, i (i)}
					<Command.Item value={key(sec, i)} onSelect={() => go(h.href)}>
						<span class="min-w-0 flex-1 truncate">{h.title}</span>
						{#if h.detail}<span class="max-w-[50%] shrink-0 truncate text-xs text-muted-foreground"
								>{h.detail}</span
							>{/if}
					</Command.Item>
				{/each}
				{#if sec.more > 0}<p class="px-2 py-1 text-xs text-muted-foreground">
						{sec.more} more
					</p>{/if}
			</Command.Group>
		{/each}
		{#if waiting}<p class="px-4 py-2 text-xs text-muted-foreground" role="status">
				Searching…
			</p>{/if}
		{#if failed}<p class="px-4 py-2 text-xs text-bad-text" role="status">
				Stored objects couldn't be searched. Pages and settings still are.
			</p>{/if}
		{#if nothing}<p class="px-4 py-6 text-center text-md text-muted-foreground">
				Nothing matches <Mono>{q}</Mono>.
			</p>{/if}
	</Command.List>
</Command.Dialog>
