<script lang="ts">
	// One Badges button on the service page (07 §7.3): it opens every badge this entry has, each shown once as the real
	// image the status listener serves, as it will look in a README. A click copies it as Markdown or as a plain link.
	import * as Popover from '#lib/components/ui/popover/index.ts';
	import * as ToggleGroup from '#lib/components/ui/toggle-group/index.ts';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { BadgeCheck, Check, Copy } from '@lucide/svelte';

	let { name, slug, kinds }: { name: string; slug: string; kinds: string[] } = $props();
	let as = $state('markdown');
	let copied = $state('');
	let failed = $state('');

	const path = (kind: string) => `/badge/${slug}/${kind}.svg`;
	async function copy(kind: string) {
		const url = location.origin + path(kind);
		try {
			await navigator.clipboard.writeText(as === 'markdown' ? `![${name} ${kind}](${url})` : url);
			copied = kind;
			failed = '';
		} catch {
			failed = kind; // no clipboard here (an address without HTTPS, or the browser said no)
		}
		setTimeout(() => (copied = failed = ''), 2000);
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
			{#each kinds as kind (kind)}
				<li>
					<button
						type="button"
						onclick={() => copy(kind)}
						aria-label="Copy the {kind.replace('-', ' ')} badge"
						class="group flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-left hover:bg-surface-2"
					>
						<img src={path(kind)} alt="" class="h-5 max-w-full" />
						<span
							class="ml-auto flex shrink-0 items-center text-sm text-muted-foreground group-hover:text-foreground"
							aria-live="polite"
						>
							{#if copied === kind}<span class="flex items-center gap-1 text-ok-text"
									><Check class="size-4" />Copied</span
								>{:else if failed === kind}<span class="text-warn-text">Could not copy</span
								>{:else}<Copy class="size-4" />{/if}
						</span>
					</button>
				</li>
			{/each}
		</ul>
	</Popover.Content>
</Popover.Root>
