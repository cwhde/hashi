<script lang="ts">
	// An app's icon on the public dashboard (07 §9.1, bug 149): a selfh.st icon found by search, an upload, or the one
	// Hashi finds on the app. A choice applies at once, like the dashboard switch next to it (10c §5.1).
	import * as Dialog from '#lib/components/ui/dialog/index.ts';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { RefreshCw, Search, Upload } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import AppIcon from './AppIcon.svelte';

	let {
		app,
		onchange,
		size = 'size-7 text-xs'
	}: {
		app: Schemas['AppIcon'];
		onchange: (a: Schemas['AppIcon']) => void;
		/** The trigger icon's size classes (a dense outline row uses a smaller one). */
		size?: string;
	} = $props();

	let open = $state(false);
	let query = $state('');
	let hits = $state<Schemas['IconLibraryHit'][]>([]);
	let searching = $state(false);
	let busy = $state(false);
	let error = $state('');

	let seq = 0;
	let timer: ReturnType<typeof setTimeout> | undefined;
	async function search(q: string) {
		const mine = ++seq;
		searching = true;
		const r = await api.GET('/api/icon-library', { params: { query: { q } } });
		if (mine !== seq) return;
		searching = false;
		if (r.data) {
			hits = r.data.icons;
			error = '';
		} else error = problemText(r.error);
	}
	function typed() {
		clearTimeout(timer);
		timer = setTimeout(() => void search(query), 200);
	}
	function start(o: boolean) {
		open = o;
		if (!o) return;
		// Start from the app's own name: most apps are in the collection under it.
		query = app.name.replace(/\s*\(.*\)\s*$/, '');
		error = '';
		void search(query);
	}

	async function choose(body: Schemas['AppIconChoice']) {
		busy = true;
		error = '';
		const r = await api.PUT('/api/app-icons/{app}', { params: { path: { app: app.app } }, body });
		busy = false;
		if (!r.data) {
			error = problemText(r.error);
			return;
		}
		onchange(r.data);
		if (body.mode !== 'auto' || r.data.icon) open = false;
	}
	function upload(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		const f = input.files?.[0];
		input.value = '';
		if (!f) return;
		if (f.size > 1024 * 1024) {
			error = 'At most 1 MB';
			return;
		}
		const r = new FileReader();
		r.onload = () =>
			void choose({ mode: 'upload', data: String(r.result).replace(/^data:[^,]*,/, '') });
		r.readAsDataURL(f);
	}
	const from = $derived(
		app.mode === 'library'
			? `selfh.st: ${app.library_ref}`
			: app.mode === 'upload'
				? 'Uploaded'
				: app.icon
					? `Found on the app: ${app.found}`
					: app.checked_at
						? 'None found on the app'
						: 'Not looked for yet'
	);
</script>

<button
	class="rounded-lg focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
	onclick={() => start(true)}
	aria-label="Icon of {app.name}"
>
	<AppIcon name={app.name} icon={app.icon} light={app.icon_light} class={size} />
</button>

<Dialog.Root bind:open={() => open, start}>
	<Dialog.Content class="sm:max-w-2xl">
		<Dialog.Header>
			<Dialog.Title>Icon of {app.name}</Dialog.Title>
		</Dialog.Header>
		<div class="flex items-center gap-3 text-md">
			<AppIcon name={app.name} icon={app.icon} light={app.icon_light} class="size-10 text-lg" />
			<span class="min-w-0 flex-1 truncate text-muted-foreground">{from}</span>
			<Button variant="outline" size="sm" disabled={busy} onclick={() => choose({ mode: 'auto' })}
				><RefreshCw />{app.mode === 'auto' && app.checked_at
					? 'Look again'
					: 'Find it on the app'}</Button
			>
			<Button variant="outline" size="sm" class="relative" disabled={busy}>
				<Upload />Upload
				<input
					type="file"
					accept="image/png,image/webp,image/svg+xml,image/x-icon,.ico"
					class="absolute inset-0 cursor-pointer opacity-0"
					aria-label="Upload an icon for {app.name}"
					onchange={upload}
				/>
			</Button>
		</div>
		{#if error}<p role="alert" class="text-md text-bad-text">{error}</p>{/if}
		<div class="relative">
			<Search
				class="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
			/>
			<Input
				bind:value={query}
				oninput={typed}
				placeholder="Search icons"
				aria-label="Search icons"
				class="pl-8"
			/>
		</div>
		<div class="max-h-[50vh] overflow-y-auto" aria-busy={searching}>
			{#if hits.length}
				<ul class="grid grid-cols-3 gap-2 sm:grid-cols-5" aria-label="Icons">
					{#each hits as h (h.reference)}
						<li>
							<button
								class="flex w-full flex-col items-center gap-1.5 rounded-xl border p-2 hover:border-muted-foreground/40 disabled:opacity-50"
								disabled={busy}
								onclick={() => choose({ mode: 'library', library_ref: h.reference })}
							>
								<AppIcon
									name={h.name}
									icon={h.url}
									light={h.light_url}
									class="size-10 border-0 bg-transparent"
								/>
								<span class="w-full truncate text-center text-sm">{h.name}</span>
							</button>
						</li>
					{/each}
				</ul>
			{:else if !searching}
				<p class="text-md text-muted-foreground">No icons match.</p>
			{/if}
		</div>
		<p class="text-sm text-muted-foreground">
			Icons by <a href="https://selfh.st/icons/" target="_blank" rel="noreferrer" class="underline"
				>selfh.st</a
			>, licensed under
			<a
				href="https://creativecommons.org/licenses/by/4.0/"
				target="_blank"
				rel="noreferrer"
				class="underline">CC BY 4.0</a
			>.
		</p>
	</Dialog.Content>
</Dialog.Root>
