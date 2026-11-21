<script lang="ts">
	// Idea mock: "Copy badge" opens a small preview of every badge; a click copies its link (or Markdown).
	import * as Popover from '#lib/components/ui/popover/index.ts';
	import * as ToggleGroup from '#lib/components/ui/toggle-group/index.ts';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Check, Copy, Link } from '@lucide/svelte';
	import { badges } from './ideas.ts';

	let { slug }: { slug: string } = $props();
	let period = $state('30d');
	let as = $state('markdown');
	let copied = $state('');

	const url = (id: string) =>
		`https://status.example.com/badge/${slug}/${id}${id === 'uptime' || id === 'response' || id === 'chart' ? '-' + period : ''}.svg`;
	function copy(id: string, name: string) {
		const u = url(id);
		void navigator.clipboard?.writeText(as === 'markdown' ? `![${name}](${u})` : u);
		copied = id;
		setTimeout(() => (copied = ''), 1500);
	}
	const width = (s: string) => 12 + s.length * 6.6;
</script>

<Popover.Root>
	<Popover.Trigger>
		{#snippet child({ props })}
			<Button {...props} variant="outline" size="sm"><Link />Badges</Button>
		{/snippet}
	</Popover.Trigger>
	<Popover.Content class="w-[min(24rem,calc(100vw-2rem))] p-0" align="end">
		<div class="flex items-center gap-2 border-b px-3 py-2">
			<ToggleGroup.Root type="single" bind:value={period} variant="outline" size="sm">
				{#each ['24h', '7d', '30d'] as p (p)}<ToggleGroup.Item value={p}>{p}</ToggleGroup.Item
					>{/each}
			</ToggleGroup.Root>
			<span class="flex-1"></span>
			<ToggleGroup.Root type="single" bind:value={as} variant="outline" size="sm">
				<ToggleGroup.Item value="markdown">Markdown</ToggleGroup.Item>
				<ToggleGroup.Item value="link">Link</ToggleGroup.Item>
			</ToggleGroup.Root>
		</div>
		<ul class="divide-y">
			{#each badges as b (b.id)}
				{@const label = b.label.replace('30d', period).replace('24h', period)}
				<li>
					<button
						type="button"
						onclick={() => copy(b.id, b.name)}
						class="flex w-full items-center gap-3 px-3 py-2.5 text-left hover:bg-surface-2"
					>
						<span class="min-w-0 flex-1">
							<span class="block text-md font-medium">{b.name}</span>
							<span class="block text-sm text-muted-foreground">{b.about}</span>
						</span>
						{#if b.tone === 'chart'}
							<svg
								viewBox="0 0 120 28"
								class="h-7 w-[120px] rounded-sm bg-surface-2"
								aria-hidden="true"
							>
								<polyline
									points="2,20 14,16 26,18 38,10 50,14 62,12 74,19 86,8 98,13 110,11 118,15"
									class="fill-none stroke-ok"
									stroke-width="1.5"
								/>
							</svg>
						{:else}
							<svg
								viewBox="0 0 {width(label) + width(b.value)} 20"
								class="h-5 shrink-0"
								style="width:{width(label) + width(b.value)}px"
								aria-label="{label}: {b.value}"
							>
								<rect width={width(label)} height="20" rx="3" class="fill-muted-foreground/70" />
								<rect x={width(label)} width={width(b.value)} height="20" rx="3" class="fill-ok" />
								<rect x={width(label)} width="4" height="20" class="fill-ok" />
								<text x="5" y="14" class="fill-white text-[10.5px]">{label}</text>
								<text x={width(label) + 5} y="14" class="fill-white text-[10.5px]">{b.value}</text>
							</svg>
						{/if}
						<span class="grid size-6 place-items-center text-muted-foreground">
							{#if copied === b.id}<Check class="size-4 text-ok-text" />{:else}<Copy
									class="size-4"
								/>{/if}
						</span>
					</button>
				</li>
			{/each}
		</ul>
	</Popover.Content>
</Popover.Root>
