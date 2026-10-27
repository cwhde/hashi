<script lang="ts">
	import { nav } from '$lib/design/nav';
	import { theme, themes } from '$lib/theme.svelte';
	import { cn } from '$lib/utils';

	// S0b deliverable index: every page, grouped as in the navigation, plus pages outside the admin shell.
	const notes: Record<string, string> = {
		'/design/overview':
			'Pending plans first, then health, hosts, sync, security, certificates, activity',
		'/design/resources':
			'Dense table, add-resource drawer · detail: summary strip + Routing / Access / Rules / Monitoring / Config tabs',
		'/design/dns':
			'Inline row editing, generated toggle, import decisions, AdGuard, internal agent names',
		'/design/traefik': 'Hosts with files and updates, middleware editor, certificates',
		'/design/firewall': 'Hosts, allowlist · detail: rescue, NetBird, ports, live ruleset, script',
		'/design/pulse': 'Agents table, install drawer',
		'/design/status':
			'Grouped endpoint cards with history strips, notifications · detail with charts',
		'/design/app-display': 'What the public dashboard and status page show',
		'/design/security':
			'Dashboard, blocks, blocklists, challenge, sign-in providers, policies · subject detail with decision trace',
		'/design/scripts': 'Editor, targets, schedule, run history',
		'/design/activity': 'Audit log, jobs, live log',
		'/design/connections': 'Every external system with health',
		'/design/settings': 'All categories; Appearance has themes, custom theme editor, widgets'
	};
	const extra = [
		{
			label: 'Pending changes',
			href: '/design/changes',
			note: 'Full plan review: exact before/after values, risk, passkey approval'
		},
		{
			label: 'States',
			href: '/design/states',
			note: 'Vault locked, empty, loading, error, confirmations'
		},
		{ label: 'Sign in', href: '/design/login', note: 'Passkey only' },
		{
			label: 'Setup',
			href: '/design/setup',
			note: '11 resumable steps; shown at step 5 with host-key pinning'
		},
		{ label: 'Public app dashboard', href: '/design/public-apps', note: 'Port 8081' },
		{ label: 'Public status page', href: '/design/public-status', note: 'Port 8082' },
		{
			label: 'Visitor challenge',
			href: '/design/challenge',
			note: 'Shown by the edge when traffic looks abusive'
		}
	];
</script>

<svelte:head><title>Hashi UI design</title></svelte:head>
<div class="mx-auto max-w-5xl px-4 py-10 md:py-14">
	<div class="flex items-center gap-3">
		<span
			class="grid size-9 place-items-center rounded-xl bg-primary text-[18px] font-semibold text-primary-foreground"
			>橋</span
		>
		<h1 class="text-[24px] font-semibold">Hashi UI design</h1>
	</div>
	<p class="mt-2 max-w-[68ch] text-[14px] text-muted-foreground">
		Every page of the finished product as a clickable mock with sample data. Feature slices fill
		these pages with real data; the rules behind them are in <span class="font-mono text-[13px]"
			>docs/spec/10b-ui-system.md</span
		>.
	</p>

	<div class="mt-6 flex flex-wrap gap-2" role="radiogroup" aria-label="Theme">
		{#each themes as t (t.id)}
			<button
				role="radio"
				aria-checked={theme.current === t.id}
				onclick={() => theme.set(t.id)}
				class={cn(
					'flex h-9 items-center gap-2 rounded-[10px] border px-3 text-[13px]',
					theme.current === t.id
						? 'bg-surface-2 font-medium'
						: 'text-muted-foreground hover:text-foreground'
				)}
			>
				<span
					data-theme={t.id}
					class="flex h-4 w-6 items-center justify-end rounded-sm border bg-card px-0.5"
					><span class="size-2 rounded-full bg-primary"></span></span
				>{t.name}
			</button>
		{/each}
	</div>

	<div class="mt-8 grid gap-6 md:grid-cols-2">
		{#each nav as section, i (i)}
			<section>
				<h2 class="mb-2 text-[13px] font-medium text-muted-foreground">
					{section.group ?? 'Start'}
				</h2>
				<ul class="divide-y rounded-2xl border bg-card">
					{#each section.items as item (item.href)}
						<li>
							<a
								href={item.href}
								class="flex gap-3 px-4 py-3 first:rounded-t-2xl last:rounded-b-2xl hover:bg-surface-2/50"
							>
								<item.icon
									class="mt-0.5 size-[18px] shrink-0 text-muted-foreground"
									strokeWidth={1.75}
								/>
								<span
									><span class="block text-[14px] font-medium">{item.label}</span><span
										class="block text-[12.5px] text-muted-foreground">{notes[item.href]}</span
									></span
								>
							</a>
						</li>
					{/each}
				</ul>
			</section>
		{/each}
		<section>
			<h2 class="mb-2 text-[13px] font-medium text-muted-foreground">Flows and other surfaces</h2>
			<ul class="divide-y rounded-2xl border bg-card">
				{#each extra as e (e.href)}
					<li>
						<a
							href={e.href}
							class="block px-4 py-3 first:rounded-t-2xl last:rounded-b-2xl hover:bg-surface-2/50"
							><span class="block text-[14px] font-medium">{e.label}</span><span
								class="block text-[12.5px] text-muted-foreground">{e.note}</span
							></a
						>
					</li>
				{/each}
			</ul>
		</section>
	</div>
</div>
