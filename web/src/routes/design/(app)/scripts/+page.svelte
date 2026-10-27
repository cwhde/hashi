<script lang="ts">
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Checkbox } from '$lib/components/ui/checkbox';
	import { scripts } from '$lib/design/mock';
	import { cn } from '$lib/utils';
	import { Plus, Play, Fingerprint } from '@lucide/svelte';

	let selected = $state('s2');
	const s = $derived(scripts.find((x) => x.id === selected)!);
</script>

<PageHeader
	title="Scripts"
	description="Shell scripts Hashi keeps on your firewall hosts and runs on a schedule or by hand. They run as root, so saving and running ask for your passkey."
>
	{#snippet actions()}<Button><Plus />New script</Button>{/snippet}
</PageHeader>

<div class="grid gap-4 lg:grid-cols-[18rem_minmax(0,1fr)]">
	<nav class="h-fit rounded-2xl border bg-card p-1.5" aria-label="Scripts">
		{#each scripts as x (x.id)}
			<button
				onclick={() => (selected = x.id)}
				class={cn(
					'w-full rounded-xl px-3 py-2.5 text-left',
					selected === x.id ? 'bg-surface-2' : 'hover:bg-surface-2/50'
				)}
			>
				<div class="text-[13.5px] font-medium">{x.name}</div>
				<div class="mt-1 flex items-center gap-1" aria-label="Last runs">
					{#each x.runs as r, i (i)}<span
							class={cn('h-3 w-1.5 rounded-sm', r === 'ok' ? 'bg-ok/80' : 'bg-bad')}
						></span>{/each}
					<span class="ml-1.5 text-[12px] text-muted-foreground">{x.schedule}</span>
				</div>
			</button>
		{/each}
	</nav>
	<div class="space-y-4">
		<Panel title={s.name} description="Last run {s.last}" flush>
			{#snippet actions()}<Button size="sm" variant="outline"><Play />Run now</Button><Button
					size="sm"><Fingerprint />Save</Button
				>{/snippet}
			<pre
				class="overflow-x-auto border-b bg-surface-2/40 p-4 font-mono text-[12.5px] leading-relaxed"><code
					><span class="text-muted-foreground">1</span>  #!/bin/sh
<span class="text-muted-foreground">2</span>  set -eu
<span class="text-muted-foreground">3</span>  zfs list -H -t snapshot -o name -s creation \
<span class="text-muted-foreground">4</span
					>    | grep "@auto-" | head -n -"$KEEP" | xargs -r -n1 zfs destroy</code
				></pre>
			<div class="grid gap-4 p-4 md:grid-cols-3">
				<div class="space-y-1.5">
					<Label>Schedule (cron)</Label><Input value={s.schedule} class="font-mono" />
					<p class="text-[12px] text-muted-foreground">Sundays 03:30 host time</p>
				</div>
				<div class="space-y-1.5"><Label>Timeout</Label><Input value="10 min" /></div>
				<div class="space-y-1.5">
					<Label>Environment</Label>
					<div class="text-[13px]"><Mono>KEEP=14</Mono></div>
				</div>
			</div>
			<fieldset class="border-t px-4 py-3">
				<legend class="sr-only">Target hosts</legend>
				<div class="flex flex-wrap gap-4 text-[13px]">
					<span class="text-muted-foreground">Runs on</span><label class="flex items-center gap-2"
						><Checkbox />edge-fra</label
					><label class="flex items-center gap-2"><Checkbox checked />edge-home</label>
				</div>
			</fieldset>
		</Panel>
		<Panel title="Runs" flush>
			<ul class="divide-y text-[13px]">
				<li class="px-4 py-2.5">
					<div class="flex items-center gap-3">
						<Tag tone="bad">failed</Tag><span class="flex-1"
							>Sun 03:30 · edge-home · exit 1 · 2.1 s</span
						><span class="text-muted-foreground">scheduled</span>
					</div>
					<pre
						class="mt-2 rounded-lg bg-surface-2/60 p-2.5 font-mono text-[12px]">cannot destroy 'tank/media@auto-2026-09-01': dataset is busy</pre>
				</li>
				<li class="flex items-center gap-3 px-4 py-2.5">
					<Tag tone="ok">ok</Tag><span class="flex-1">Sun 20 Oct 03:30 · edge-home · 1.4 s</span
					><span class="text-muted-foreground">scheduled</span>
				</li>
			</ul>
		</Panel>
	</div>
</div>
