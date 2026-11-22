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
	// The clipboard API needs HTTPS; on a plain-http address (a LAN check) the older copy command still works.
	function fallback(text: string): boolean {
		const area = document.createElement('textarea');
		area.value = text;
		area.setAttribute('readonly', '');
		area.style.cssText = 'position:fixed;top:0;left:0;opacity:0';
		document.body.append(area);
		area.select();
		try {
			return document.execCommand('copy');
		} catch {
			return false;
		} finally {
			area.remove();
		}
	}
	let reset: ReturnType<typeof setTimeout> | undefined;
	async function copy(kind: string) {
		const url = location.origin + path(kind);
		const text = as === 'markdown' ? `![${name} ${kind}](${url})` : url;
		let ok: boolean;
		try {
			await navigator.clipboard.writeText(text);
			ok = true;
		} catch {
			ok = fallback(text);
		}
		copied = ok ? kind : '';
		failed = ok ? '' : kind;
		clearTimeout(reset);
		reset = setTimeout(() => (copied = failed = ''), 2000);
	}
</script>

<Popover.Root>
	<Popover.Trigger>
		{#snippet child({ props })}
			<Button {...props} variant="outline" size="sm"><BadgeCheck />Badges</Button>
		{/snippet}
	</Popover.Trigger>
	<Popover.Content class="w-[min(22rem,calc(100vw-2rem))] p-0" align="end">
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
						<img src={path(kind)} alt="" class="h-5 max-w-full min-w-0" />
						<!-- All three states share one spot and only the current one is visible, so the spot is as wide as the
						     longest message and nothing moves when it changes. -->
						<span
							class="ml-auto grid shrink-0 text-sm whitespace-nowrap text-muted-foreground group-hover:text-foreground"
							aria-live="polite"
						>
							<span
								class="col-start-1 row-start-1 flex items-center justify-end {copied === kind ||
								failed === kind
									? 'invisible'
									: ''}"><Copy class="size-4" /></span
							>
							<span
								class="col-start-1 row-start-1 flex items-center justify-end gap-1 text-ok-text {copied ===
								kind
									? ''
									: 'invisible'}"><Check class="size-4" />Copied</span
							>
							<span
								class="col-start-1 row-start-1 flex items-center justify-end text-warn-text {failed ===
								kind
									? ''
									: 'invisible'}">Could not copy</span
							>
						</span>
					</button>
				</li>
			{/each}
		</ul>
	</Popover.Content>
</Popover.Root>
