<script lang="ts">
	import { onMount } from 'svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import CodeEditor from '$lib/components/hashi/CodeEditor.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Switch } from '$lib/components/ui/switch';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import { CircleAlert, CircleCheck } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { pendingPlans } from '$lib/sync-pending.svelte';
	import { saveText } from '$lib/services';

	// 30-user-middlewares.yml (04 §4): the YAML editor, the middlewares it defines with where each is used, and the
	// Traefik setting "on every service" (09 §2). Every change is checked by the server as you type; a file that fails
	// the check cannot be saved, so the hosts keep the last applied one.
	type Mw = Schemas['UserMiddleware'];
	let text = $state('');
	let saved = $state('');
	let found = $state<Mw[]>([]);
	let every = $state<string[]>([]);
	let savedEvery = $state<string[]>([]);
	let problem = $state<{ line: number; message: string } | null>(null);
	let checking = $state(false);
	let loaded = $state(false);
	let loadError = $state('');
	let saveError = $state('');
	let busy = $state(false);
	let outcome = $state<{ text: string; review: boolean; bad: boolean } | null>(null);

	const dirty = $derived(text !== saved || every.join() !== savedEvery.join());

	async function load() {
		const r = await api.GET('/api/traefik/user-middlewares');
		if (r.error || !r.data) throw r.error;
		text = saved = r.data.yaml;
		found = r.data.middlewares;
		every = [...r.data.every_service];
		savedEvery = [...r.data.every_service];
		problem = null;
		loaded = true;
	}
	onMount(() => {
		load().catch((e) => (loadError = problemText(e)));
	});

	let timer: ReturnType<typeof setTimeout> | undefined;
	let seq = 0;
	async function check(yaml: string) {
		const mine = ++seq;
		checking = true;
		const r = await api.POST('/api/traefik/user-middlewares/check', { body: { yaml } });
		if (mine !== seq) return; // a newer check is on its way
		checking = false;
		if (r.error || !r.data) {
			problem = { line: 0, message: problemText(r.error) };
			return;
		}
		problem = r.data.ok ? null : { line: r.data.line ?? 0, message: r.data.error ?? '' };
		if (r.data.ok) {
			found = r.data.middlewares;
			every = every.filter((n) => found.some((m) => m.name === n));
		}
	}
	$effect(() => {
		const yaml = text;
		if (!loaded || yaml === saved) return;
		clearTimeout(timer);
		timer = setTimeout(() => check(yaml), 350);
		return () => clearTimeout(timer);
	});

	function toggleEvery(name: string, on: boolean) {
		every = on ? [...every, name] : every.filter((n) => n !== name);
	}

	async function save() {
		busy = true;
		saveError = '';
		outcome = null;
		try {
			const r = await api.PUT('/api/traefik/user-middlewares', {
				body: { yaml: text, every_service: every }
			});
			if (r.error || !r.data) throw r.error;
			outcome = saveText(r.data);
			text = saved = r.data.file.yaml;
			found = r.data.file.middlewares;
			every = [...r.data.file.every_service];
			savedEvery = [...every];
			await pendingPlans.refresh();
		} catch (e) {
			// What you typed stays in the editor; what is stored (maybe the file put back after Traefik refused the new
			// one) becomes the saved state.
			saveError = problemText(e);
			const r = await api.GET('/api/traefik/user-middlewares');
			if (r.data) {
				saved = r.data.yaml;
				savedEvery = [...r.data.every_service];
			}
		} finally {
			busy = false;
		}
	}

	const usedText = (m: Mw) => (every.includes(m.name) ? 'on every service' : 'unused');
</script>

{#if loadError}
	<p
		role="alert"
		class="flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-4 text-[13px] text-bad-text"
	>
		<CircleAlert class="mt-0.5 size-4 shrink-0" />{loadError}
	</p>
{:else if !loaded}
	<Skeleton class="h-80 rounded-2xl" />
{:else}
	{#if outcome}
		<p
			role={outcome.bad ? 'alert' : 'status'}
			class="mb-4 flex flex-wrap items-center gap-2 rounded-2xl border p-3 text-[13px] {outcome.bad
				? 'border-bad/30 bg-bad/10 text-bad-text'
				: 'bg-card'}"
		>
			{outcome.text}
			{#if outcome.review}<a href="/changes" class="font-medium underline underline-offset-2"
					>Review</a
				>{/if}
		</p>
	{/if}
	{#if saveError}
		<p
			role="alert"
			class="mb-4 flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-3 text-[13px] text-bad-text"
		>
			<CircleAlert class="mt-0.5 size-4 shrink-0" />{saveError}
		</p>
	{/if}
	<Panel
		title="30-user-middlewares.yml"
		description="Switch them on per service or route on the service pages"
		flush
	>
		{#snippet actions()}<Button
				size="sm"
				onclick={save}
				disabled={busy || !dirty || !!problem || checking}
				>{busy ? 'Applying…' : 'Save and plan'}</Button
			>{/snippet}
		<div class="grid lg:grid-cols-[minmax(0,1fr)_17rem]">
			<div class="min-w-0 bg-surface-2/40 lg:rounded-bl-2xl">
				<CodeEditor
					bind:value={text}
					label="User middleware file"
					{problem}
					class="max-h-[32rem] overflow-auto"
				/>
				<div
					class="flex min-h-10 items-start gap-2 border-t px-4 py-2.5 text-[12.5px]"
					aria-live="polite"
				>
					{#if problem}
						<CircleAlert class="mt-0.5 size-4 shrink-0 text-bad-text" />
						<span class="text-bad-text"
							>{problem.line > 0 ? `Line ${problem.line}: ` : ''}{problem.message}</span
						>
					{:else if checking}
						<span class="text-muted-foreground">Checking…</span>
					{:else}
						<CircleCheck class="mt-0.5 size-4 shrink-0 text-ok-text" />
						<span class="text-muted-foreground"
							>{dirty ? 'Valid. Not saved yet.' : 'Valid and saved.'}</span
						>
					{/if}
				</div>
			</div>
			<div class="border-t p-4 lg:border-t-0 lg:border-l">
				<div class="text-[12px] font-medium">
					{found.length === 0
						? 'No middlewares yet'
						: `Found ${found.length} middleware${found.length === 1 ? '' : 's'}`}
				</div>
				{#if found.length === 0}
					<p class="mt-2 text-[12.5px] text-muted-foreground">
						Start with <Mono>http:</Mono>, then <Mono>middlewares:</Mono>, then a name and one
						Traefik middleware type, for example <Mono>ipAllowList</Mono> or <Mono>buffering</Mono>.
					</p>
				{:else}
					<ul class="mt-2 space-y-3 text-[13px]" aria-label="Middlewares in the file">
						{#each found as m (m.name)}
							<li class="space-y-1.5">
								<div class="flex flex-wrap items-center gap-x-2 gap-y-1">
									<Mono>{m.name}</Mono>
									<Tag>{m.type}</Tag>
								</div>
								<div class="text-[12.5px] text-muted-foreground">
									{#if m.used_by.length && !every.includes(m.name)}
										used by
										{#each [...new Map(m.used_by.map( (u) => [u.service_id, u] )).values()] as u, i (u.service_id)}{i
												? ', '
												: ''}<a class="underline underline-offset-2" href="/services/{u.service_id}"
												>{u.service_name}</a
											>{/each}
									{:else}{usedText(m)}{/if}
								</div>
								<label class="flex items-center justify-between gap-3 text-[12.5px]">
									<span>On every service</span>
									<Switch
										checked={every.includes(m.name)}
										onCheckedChange={(on) => toggleEvery(m.name, on)}
										aria-label="Use {m.name} on every service"
									/>
								</label>
							</li>
						{/each}
					</ul>
					<p class="mt-4 text-[12px] text-muted-foreground">
						A file that fails the check is never saved; the hosts keep the last applied one. Your
						middlewares run after Hashi's sign-in and access checks, never before. The Hashi admin
						service never gets them.
					</p>
				{/if}
			</div>
		</div>
	</Panel>
{/if}
