<script lang="ts">
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { theme, themes } from '$lib/theme.svelte';
	import { contrast } from '$lib/design/contrast';
	import { settingsCatalogue } from '$lib/design/settings';
	import { cn } from '$lib/utils';
	import {
		Fingerprint,
		KeyRound,
		Trash2,
		GripVertical,
		Download,
		Upload,
		Lock
	} from '@lucide/svelte';

	const cats = [
		'General',
		'Account',
		'Appearance',
		'Security',
		'Monitoring',
		'Dashboard',
		'DNS',
		'Traefik',
		'Firewall',
		'Notifications',
		'Pulse',
		'Internal agent DNS'
	];
	let cat = $state('Appearance');

	// Custom theme draft: previewed on a scoped element, never applied globally until saved.
	let draft = $state({
		canvas: '#0B0D10',
		surface: '#14171C',
		text: '#EEF1F4',
		muted: '#8E98A6',
		border: '#2A313B',
		accent: '#14B8A6'
	});
	const ratios = $derived({
		text: contrast(draft.text, draft.surface),
		muted: contrast(draft.muted, draft.surface)
	});
	const readable = $derived(ratios.text >= 4.5 && ratios.muted >= 4.5);
	const widgets = [
		'Pending changes',
		'Needs attention',
		'Resource health',
		'Edge hosts',
		'Sync state',
		'Security',
		'Certificates',
		'Recent activity',
		'Vault state',
		'Recent incidents'
	];
</script>

<PageHeader
	title="Settings"
	description="Every setting here changes behaviour somewhere; nothing is saved that failed to load."
/>
<div class="grid gap-5 lg:grid-cols-[14rem_minmax(0,1fr)]">
	<nav aria-label="Settings categories" class="flex gap-1 overflow-x-auto lg:flex-col">
		{#each cats as c (c)}
			<button
				onclick={() => (cat = c)}
				aria-current={cat === c ? 'page' : undefined}
				class={cn(
					'h-9 shrink-0 rounded-[10px] px-3 text-left text-[13.5px]',
					cat === c ? 'bg-surface-2 font-medium' : 'text-muted-foreground hover:text-foreground'
				)}>{c}</button
			>
		{/each}
	</nav>

	<div class="min-w-0 space-y-4">
		{#if cat === 'Appearance'}
			<Panel title="Theme">
				<div class="grid grid-cols-2 gap-3 md:grid-cols-4">
					{#each themes as t (t.id)}
						<button
							onclick={() => theme.set(t.id)}
							aria-pressed={theme.current === t.id}
							class={cn(
								'rounded-xl border p-1.5 text-left',
								theme.current === t.id && 'ring-2 ring-ring'
							)}
						>
							<div
								data-theme={t.id}
								data-mode={t.mode}
								class="flex h-20 gap-1.5 rounded-lg border bg-background p-1.5"
							>
								<div class="w-5 rounded-md bg-sidebar"></div>
								<div class="flex flex-1 flex-col justify-between rounded-md border bg-card p-1.5">
									<div class="h-1.5 w-3/4 rounded-full bg-muted-foreground/40"></div>
									<div class="h-2.5 w-8 self-end rounded bg-primary"></div>
								</div>
							</div>
							<div class="px-1 pt-1.5 text-[13px] font-medium">{t.name}</div>
						</button>
					{/each}
				</div>
			</Panel>

			<Panel
				title="Custom theme"
				description="Preview before saving. Hashi refuses themes with unreadable text."
			>
				{#snippet actions()}<Button variant="ghost" size="sm"><Upload />Import</Button><Button
						variant="ghost"
						size="sm"><Download />Export</Button
					>{/snippet}
				<div class="grid gap-5 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
					<div class="grid grid-cols-2 gap-3">
						{#each Object.keys(draft) as k (k)}
							<label class="space-y-1.5"
								><span class="text-[13px] font-medium capitalize">{k}</span>
								<span class="flex items-center gap-2"
									><input
										type="color"
										bind:value={draft[k as keyof typeof draft]}
										class="size-8 shrink-0 cursor-pointer rounded-md border bg-transparent"
										aria-label="{k} colour"
									/><Input
										bind:value={draft[k as keyof typeof draft]}
										class="font-mono"
										aria-label="{k} hex"
									/></span
								>
							</label>
						{/each}
					</div>
					<div>
						<div class="rounded-xl border p-3" style="background:{draft.canvas}">
							<div
								class="rounded-lg border p-3"
								style="background:{draft.surface};border-color:{draft.border};color:{draft.text}"
							>
								<div class="text-[14px] font-semibold">Grafana</div>
								<div class="text-[12.5px]" style="color:{draft.muted}">
									grafana.example.com · 48 ms
								</div>
								<div class="mt-3 flex justify-end">
									<span
										class="rounded-md px-2.5 py-1 text-[12.5px] font-medium"
										style="background:{draft.accent};color:{contrast(draft.accent, '#000000') > 7
											? '#000'
											: '#fff'}">Save changes</span
									>
								</div>
							</div>
						</div>
						<dl class="mt-3 grid grid-cols-2 gap-2 text-[13px]">
							<dt>Text on surface</dt>
							<dd class={ratios.text >= 4.5 ? 'text-ok-text' : 'text-bad-text'}>
								{ratios.text.toFixed(1)} : 1
							</dd>
							<dt>Muted text on surface</dt>
							<dd class={ratios.muted >= 4.5 ? 'text-ok-text' : 'text-bad-text'}>
								{ratios.muted.toFixed(1)} : 1
							</dd>
						</dl>
						<div class="mt-3 flex justify-end gap-2">
							<Button variant="ghost" size="sm">Reset to Graphite</Button>
							<Button size="sm" disabled={!readable}
								>{readable ? 'Save theme' : 'Text too faint to save'}</Button
							>
						</div>
					</div>
				</div>
			</Panel>

			<Panel
				title="Overview widgets"
				description="Drag to reorder; switch off what you don't need"
				flush
			>
				<ul class="divide-y">
					{#each widgets as w, i (w)}<li class="flex items-center gap-3 px-4 py-2 text-[13px]">
							<GripVertical class="size-4 text-muted-foreground" /><span class="flex-1">{w}</span
							><Switch checked={i < 8} aria-label="Show {w}" />
						</li>{/each}
				</ul>
			</Panel>
			<Panel title="Comfort">
				<label class="flex items-center justify-between gap-3 text-[13px]"
					>Reduce transparency<Switch /></label
				>
			</Panel>
		{:else if cat === 'Account'}
			<Panel title="Passkeys" description="Sign-in and confirmation of risky changes" flush>
				{#snippet actions()}<Button size="sm" variant="outline"><Fingerprint />Add passkey</Button
					>{/snippet}
				<ul class="divide-y text-[13px]">
					<li class="flex items-center gap-3 px-4 py-3">
						<KeyRound class="size-4 text-muted-foreground" /><span class="flex-1"
							><span class="font-medium">MacBook</span>
							<span class="text-muted-foreground">· iCloud Keychain · last used today 12:10</span
							></span
						><Button variant="ghost" size="icon-sm" aria-label="Delete passkey MacBook"
							><Trash2 /></Button
						>
					</li>
					<li class="flex items-center gap-3 px-4 py-3">
						<KeyRound class="size-4 text-muted-foreground" /><span class="flex-1"
							><span class="font-medium">YubiKey 5C</span>
							<span class="text-muted-foreground">· security key · last used 3 d ago</span></span
						><Button variant="ghost" size="icon-sm" aria-label="Delete passkey YubiKey"
							><Trash2 /></Button
						>
					</li>
				</ul>
			</Panel>
			<Panel
				title="Active sessions"
				description="Each session is tied to the IP it started from and ends after 8 h at most"
				flush
			>
				{#snippet actions()}<Button size="sm" variant="outline">Sign out other sessions</Button
					>{/snippet}
				<ul class="divide-y text-[13px]">
					<li class="flex items-center gap-3 px-4 py-3">
						<span class="flex-1"><Mono>192.0.2.10</Mono> · started 12:10 · ends 19:10</span><Tag
							tone="accent">this session</Tag
						>
					</li>
					<li class="flex items-center gap-3 px-4 py-3">
						<span class="flex-1"><Mono>198.51.100.9</Mono> · started 08:02 · ends 16:02</span
						><Button size="xs" variant="ghost">Sign out</Button>
					</li>
				</ul>
			</Panel>
			<Panel title="Vault">
				<div class="flex items-start gap-3 text-[13px]">
					<Lock class="mt-0.5 size-4 text-muted-foreground" />
					<p class="flex-1">
						Service mode: secrets unlock automatically with the deployment's master key, so syncs
						keep running after a restart. Strict mode keeps them locked until you unlock with a
						passkey or the recovery key.
					</p>
					<Button size="sm" variant="outline">Change mode…</Button>
				</div>
			</Panel>
		{:else if cat === 'General'}
			<Panel title="Domains and addresses">
				<div class="grid gap-4 md:grid-cols-2">
					<div class="space-y-1.5"><Label>Root domain</Label><Input value="example.com" /></div>
					<div class="space-y-1.5">
						<Label>Admin domain</Label><Input value="hashi.example.com" />
					</div>
					<div class="space-y-1.5">
						<Label>Internal Hashi URL</Label><Input
							value="http://10.20.4.106:3340"
							class="font-mono"
						/>
					</div>
					<div class="space-y-1.5"><Label>Reconcile every</Label><Input value="1 hour" /></div>
				</div>
				<div class="mt-4 flex justify-end"><Button size="sm">Save</Button></div>
			</Panel>
			<Panel
				title="Trusted proxies"
				description="Forwarded client IPs are believed only from these. Changing this asks for your passkey."
			>
				<Mono>10.20.4.11/32, 10.20.1.11/32 (Traefik hosts, automatic)</Mono>
			</Panel>
		{:else}
			<Panel title={cat}>
				<div class="grid gap-x-5 gap-y-4 md:grid-cols-2">
					{#each settingsCatalogue[cat] ?? [] as f (f.label)}
						{#if f.kind === 'switch'}
							<label
								class="flex items-center justify-between gap-3 rounded-xl border px-3 py-2.5 text-[13px]"
								>{f.label}<Switch checked={f.value as boolean} /></label
							>
						{:else}
							<div class="space-y-1.5">
								<Label class="flex items-center gap-1.5"
									>{f.label}{#if f.reauth}<Fingerprint
											class="size-3.5 text-muted-foreground"
											aria-label="Changing this asks for your passkey"
										/>{/if}</Label
								>
								<Input value={String(f.value)} class={f.kind === 'mono' ? 'font-mono' : ''} />
								{#if f.hint}<p class="text-[12px] text-muted-foreground">{f.hint}</p>{/if}
							</div>
						{/if}
					{/each}
				</div>
				<div class="mt-4 flex justify-end"><Button size="sm">Save</Button></div>
			</Panel>
		{/if}
	</div>
</div>
