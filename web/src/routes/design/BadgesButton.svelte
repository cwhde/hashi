<script lang="ts">
	// Mock (07 §7.3): one Badges button on the service page; it opens every badge the status listener serves for this
	// service, shown once as the real image (badge.ts), as it will look in a README. A click
	// copies it as Markdown or as a plain link.
	import * as Popover from '#lib/components/ui/popover/index.ts';
	import * as ToggleGroup from '#lib/components/ui/toggle-group/index.ts';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { BadgeCheck, Check, Copy } from '@lucide/svelte';
	import type { Entry } from './ideas.ts';
	import { slug } from './ideas.ts';
	import { badgeSvg, badgeUri, type Tone } from './badge.ts';

	let { e }: { e: Entry } = $props();
	let as = $state('markdown');
	let copied = $state('');
	// The value's colour says how good it is (07 §7.3).
	const byUptime = (v: number): Tone => (v >= 99.5 ? 'up' : v >= 98 ? 'degraded' : 'down');
	const list = $derived.by(() => {
		const tone: Tone =
			e.state === 'up' || e.state === 'degraded' || e.state === 'down' ? e.state : 'unknown';
		const out = [
			{
				kind: 'status',
				svg: badgeSvg(e.name, e.state === 'degraded' ? 'slow' : e.state, tone, true)
			},
			{
				kind: 'uptime-24h',
				svg: badgeSvg('uptime 24h', `${e.uptime.h24}%`, byUptime(e.uptime.h24))
			},
			{ kind: 'uptime-7d', svg: badgeSvg('uptime 7d', `${e.uptime.d7}%`, byUptime(e.uptime.d7)) },
			{
				kind: 'uptime-30d',
				svg: badgeSvg('uptime 30d', `${e.uptime.d30.toFixed(2)}%`, byUptime(e.uptime.d30))
			},
			{
				kind: 'response-24h',
				svg: badgeSvg(
					'response 24h',
					`${e.ms} ms`,
					e.ms < 300 ? 'up' : e.ms < 1000 ? 'degraded' : 'down'
				)
			}
		];
		if (e.kind === 'web')
			out.push({ kind: 'certificate', svg: badgeSvg('certificate', '61 days', 'up') });
		return out;
	});
	function copy(kind: string) {
		const url = `https://status.example.com/badge/${slug(e.name)}/${kind}.svg`;
		void navigator.clipboard?.writeText(as === 'markdown' ? `![${e.name} ${kind}](${url})` : url);
		copied = kind;
		setTimeout(() => (copied = ''), 1500);
	}
</script>

<Popover.Root>
	<Popover.Trigger>
		{#snippet child({ props })}
			<Button {...props} variant="outline" size="sm"><BadgeCheck />Badges</Button>
		{/snippet}
	</Popover.Trigger>
	<Popover.Content class="w-[min(20rem,calc(100vw-2rem))] p-0" align="end">
		<div class="flex items-center justify-between gap-2 border-b px-3 py-2">
			<span class="text-md font-medium">Copy as</span>
			<ToggleGroup.Root
				type="single"
				value={as}
				onValueChange={(v) => v && (as = v)}
				variant="outline"
				size="sm"
			>
				<ToggleGroup.Item value="markdown">Markdown</ToggleGroup.Item>
				<ToggleGroup.Item value="link">Link</ToggleGroup.Item>
			</ToggleGroup.Root>
		</div>
		<ul class="p-1.5">
			{#each list as b (b.kind)}
				{@const src = badgeUri(b.svg)}
				<li>
					<button
						type="button"
						onclick={() => copy(b.kind)}
						aria-label="Copy the {b.kind.replace('-', ' ')} badge"
						class="group flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-left hover:bg-surface-2"
					>
						<img {src} alt="" class="h-5 max-w-full" />
						<span
							class="ml-auto flex items-center text-sm text-muted-foreground group-hover:text-foreground"
						>
							{#if copied === b.kind}<span class="flex items-center gap-1 text-ok-text"
									><Check class="size-4" />Copied</span
								>{:else}<Copy class="size-4" />{/if}
						</span>
					</button>
				</li>
			{/each}
		</ul>
	</Popover.Content>
</Popover.Root>
