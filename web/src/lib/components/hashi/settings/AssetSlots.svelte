<script lang="ts" module>
	import type { Schemas } from '#lib/api/client.ts';
	export type SlotName = Schemas['AssetSlotName'];
	/** A change waiting for Save: a file of the source, an upload, or emptying the slot. */
	export type Pending = { path: string } | { upload: string; size: number } | { remove: true };
	export const slotNames: { id: SlotName; label: string; empty: string; folder: string }[] = [
		{ id: 'icon', label: 'Icon', empty: 'Hashi’s icon', folder: 'icon/' },
		{ id: 'logo_dark', label: 'Logo on dark themes', empty: 'Title as text', folder: 'logo/' },
		{ id: 'logo_light', label: 'Logo on light themes', empty: 'Title as text', folder: 'logo/' },
		{ id: 'background', label: 'Background', empty: 'None', folder: 'background/' }
	];
	export const limit = (s: SlotName) => (s === 'background' ? 5 : 1) * 1024 * 1024;
</script>

<script lang="ts">
	// Settings → Public pages → Look (07 §9, bug 30): what fills each slot. Picks and uploads wait for the category's
	// Save like every other change here (10c §5.1); `files` keeps the uploads themselves out of the draft's snapshot.
	import * as Dialog from '#lib/components/ui/dialog/index.ts';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Switch } from '#lib/components/ui/switch/index.ts';
	import { ChevronLeft, Folder, Upload } from '@lucide/svelte';
	import { api, problemText } from '#lib/api/client.ts';

	let {
		look = $bindable(),
		saved,
		pending = $bindable(),
		files
	}: {
		look: Schemas['PublicAssetsSettings'];
		saved: Schemas['PublicAssets'];
		pending: Partial<Record<SlotName, Pending>>;
		files: Map<SlotName, File>;
	} = $props();

	const current = (s: SlotName) => saved.slots.find((x) => x.slot === s);
	const sourceReady = $derived(!!saved.source && saved.source === look.source.trim());
	const sourceURL = (p: string) => saved.source.replace(/\/?$/, '/') + p;

	// What the row shows: the pending change first, then what is saved.
	function preview(s: SlotName): { src?: string; text: string } {
		const p = pending[s];
		if (p && 'remove' in p) return { text: slotNames.find((x) => x.id === s)!.empty };
		if (p && 'path' in p) return { src: sourceURL(p.path), text: p.path };
		if (p && 'upload' in p) return { src: blobs[s], text: p.upload };
		const c = current(s);
		if (!c) return { text: slotNames.find((x) => x.id === s)!.empty };
		return { src: c.url, text: c.path ?? 'Uploaded' };
	}
	const filled = (s: SlotName) => {
		const p = pending[s];
		return p ? !('remove' in p) : !!current(s);
	};

	let errors = $state<Partial<Record<SlotName, string>>>({});
	// One preview address per upload, made when it is chosen and let go when it is replaced.
	let blobs = $state<Partial<Record<SlotName, string>>>({});
	function setBlob(s: SlotName, f?: File) {
		if (blobs[s]) URL.revokeObjectURL(blobs[s]);
		blobs[s] = f ? URL.createObjectURL(f) : undefined;
	}
	function upload(s: SlotName, e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		const f = input.files?.[0];
		input.value = '';
		if (!f) return;
		if (f.size > limit(s)) {
			errors[s] = `At most ${limit(s) / 1024 / 1024} MB`;
			return;
		}
		errors[s] = '';
		files.set(s, f);
		setBlob(s, f);
		pending = { ...pending, [s]: { upload: f.name, size: f.size } };
	}
	function clear(s: SlotName) {
		files.delete(s);
		setBlob(s);
		const next = { ...pending };
		if (current(s)) next[s] = { remove: true };
		else delete next[s];
		pending = next;
	}

	// Choosing a file of the source: one folder at a time, starting at the slot's usual folder.
	let browsing = $state<SlotName | null>(null);
	let folder = $state<Schemas['AssetFolder'] | null>(null);
	let folderError = $state('');
	async function open(dir: string) {
		folderError = '';
		const r = await api.GET('/api/public-assets/source', { params: { query: { dir } } });
		if (r.data) folder = r.data;
		else if (dir) await open('');
		else folderError = problemText(r.error);
	}
	function browse(s: SlotName) {
		browsing = s;
		folder = null;
		void open(slotNames.find((x) => x.id === s)!.folder);
	}
	function pick(file: string) {
		const s = browsing!;
		files.delete(s);
		setBlob(s);
		pending = { ...pending, [s]: { path: folder!.dir + file } };
		browsing = null;
	}
	const up = (dir: string) => dir.replace(/[^/]+\/$/, '');
</script>

<div class="space-y-4">
	<label class="block space-y-1.5 text-md">
		<span class="font-medium">Asset source</span>
		<Input bind:value={look.source} placeholder="https://static.example.com/" />
	</label>
	<ul class="divide-y rounded-xl border" aria-label="Public page assets">
		{#each slotNames as s (s.id)}
			{@const p = preview(s.id)}
			<li class="flex flex-wrap items-center gap-3 px-3 py-2 text-md">
				<span
					class="grid size-10 shrink-0 place-items-center overflow-hidden rounded-lg border bg-surface-2"
				>
					{#if p.src}<img src={p.src} alt="" class="size-full object-contain" />{/if}
				</span>
				<span class="min-w-0 flex-1">
					<span class="block font-medium">{s.label}</span>
					<span class="block truncate text-sm text-muted-foreground">{p.text}</span>
					{#if errors[s.id]}<span role="alert" class="block text-sm text-bad-text"
							>{errors[s.id]}</span
						>{/if}
				</span>
				<span class="flex gap-1.5">
					{#if sourceReady}<Button
							variant="outline"
							size="sm"
							onclick={() => browse(s.id)}
							aria-label="Choose {s.label} from the source"><Folder />Choose</Button
						>{/if}
					<Button variant="outline" size="sm" class="relative">
						<Upload />Upload
						<input
							type="file"
							accept="image/png,image/webp,image/svg+xml,image/x-icon,.ico"
							class="absolute inset-0 cursor-pointer opacity-0"
							aria-label="Upload {s.label}"
							onchange={(e) => upload(s.id, e)}
						/>
					</Button>
					{#if filled(s.id)}<Button
							variant="ghost"
							size="sm"
							onclick={() => clear(s.id)}
							aria-label="Remove {s.label}">Remove</Button
						>{/if}
				</span>
			</li>
		{/each}
	</ul>
	{#if filled('background')}
		<label class="flex items-center justify-between gap-3 text-md"
			>Show the background<Switch bind:checked={look.background} /></label
		>
	{/if}
</div>

<Dialog.Root open={browsing !== null} onOpenChange={(o) => !o && (browsing = null)}>
	<Dialog.Content class="sm:max-w-2xl">
		<Dialog.Header>
			<Dialog.Title>{slotNames.find((x) => x.id === browsing)?.label}</Dialog.Title>
			<Dialog.Description>{folder ? folder.dir || '/' : 'Loading…'}</Dialog.Description>
		</Dialog.Header>
		{#if folderError}<p role="alert" class="text-md text-bad-text">{folderError}</p>{/if}
		{#if folder}
			<div class="max-h-[60vh] space-y-3 overflow-y-auto">
				{#if folder.dir || folder.dirs.length}
					<div class="flex flex-wrap gap-1.5">
						{#if folder.dir}<Button variant="ghost" size="sm" onclick={() => open(up(folder!.dir))}
								><ChevronLeft />Back</Button
							>{/if}
						{#each folder.dirs as d (d)}<Button
								variant="outline"
								size="sm"
								onclick={() => open(folder!.dir + d + '/')}><Folder />{d}</Button
							>{/each}
					</div>
				{/if}
				{#if folder.files.length}
					<div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
						{#each folder.files as f (f)}
							<button
								class="space-y-1.5 rounded-xl border p-2 text-left hover:border-muted-foreground/40"
								onclick={() => pick(f)}
							>
								<span class="grid h-20 place-items-center rounded-lg bg-surface-2">
									<img
										src={sourceURL(folder.dir + f)}
										alt=""
										loading="lazy"
										class="max-h-full max-w-full object-contain"
									/>
								</span>
								<span class="block truncate text-sm">{f}</span>
							</button>
						{/each}
					</div>
				{:else}
					<p class="text-md text-muted-foreground">No images in this folder.</p>
				{/if}
			</div>
		{/if}
	</Dialog.Content>
</Dialog.Root>
