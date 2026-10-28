<script lang="ts">
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import Tag from '$lib/components/hashi/Tag.svelte';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import StatusBadge from '$lib/components/hashi/StatusBadge.svelte';
	import { Button } from '$lib/components/ui/button';
	import * as Tabs from '$lib/components/ui/tabs';
	import { traefikHosts } from '$lib/design/mock';
	import { Plus, ArrowUpCircle, FileCode2 } from '@lucide/svelte';

	const files = [
		{ f: '00-hashi-core.yml', what: 'Default middlewares', state: 'same' },
		{ f: '10-hashi-http-services.yml', what: 'HTTP routers and services', state: 'differs' },
		{ f: '20-hashi-stream-services.yml', what: 'TCP/UDP routers', state: 'same' },
		{ f: '30-user-middlewares.yml', what: 'Your middlewares', state: 'same' },
		{ f: '40-hashi-security.yml', what: 'WAF and rate limits', state: 'same' },
		{ f: '90-hashi-health.yml', what: 'Health routers', state: 'same' }
	];
</script>

<PageHeader
	title="Traefik"
	description="Every Traefik host gets every enabled service; DNS decides which one receives traffic. Hashi installs, configures and updates Traefik itself."
>
	{#snippet actions()}<Button><Plus />Add Traefik host</Button>{/snippet}
</PageHeader>

<Tabs.Root value="hosts">
	<Tabs.List class="mb-4">
		<Tabs.Trigger value="hosts">Hosts</Tabs.Trigger>
		<Tabs.Trigger value="middlewares">Your middlewares</Tabs.Trigger>
		<Tabs.Trigger value="certs">Certificates</Tabs.Trigger>
	</Tabs.List>

	<Tabs.Content value="hosts" class="space-y-4">
		{#each traefikHosts as t (t.id)}
			<Panel>
				<div class="grid gap-5 lg:grid-cols-[18rem_minmax(0,1fr)]">
					<div class="space-y-3">
						<div>
							<div class="flex items-center gap-2 text-[15px] font-semibold">
								{t.name}<StatusBadge state={t.health} />
							</div>
							<div class="text-[12.5px] text-muted-foreground">
								SSH <Mono>{t.host}</Mono> · {t.os}
							</div>
						</div>
						<dl class="grid grid-cols-2 gap-y-2 text-[13px]">
							<dt class="text-muted-foreground">Version</dt>
							<dd>{t.version}</dd>
							<dt class="text-muted-foreground">Routers</dt>
							<dd>{t.routers}, {t.errors} with errors</dd>
							<dt class="text-muted-foreground">Verified</dt>
							<dd>{t.lastVerify}</dd>
							<dt class="text-muted-foreground">Certificates</dt>
							<dd>{t.certs}</dd>
						</dl>
						<div class="rounded-xl border border-info/25 bg-info/8 p-3 text-[12.5px]">
							<div class="flex items-center gap-1.5 font-medium text-info-text">
								<ArrowUpCircle class="size-4" />Traefik {t.update} is available
							</div>
							<p class="mt-1 text-muted-foreground">
								Patch release. Upgrade swaps the binary, restarts, and rolls back on its own if
								routers fail to verify.
							</p>
							<Button size="sm" variant="outline" class="mt-2">Plan upgrade</Button>
						</div>
					</div>
					<div class="min-w-0">
						<div class="mb-1.5 text-[12px] font-medium text-muted-foreground">
							Files Hashi writes to <Mono>/etc/hashi/traefik/dynamic</Mono>
						</div>
						<ul class="divide-y rounded-xl border">
							{#each files as f (f.f)}
								<li class="flex items-center gap-3 px-3 py-2 text-[13px]">
									<FileCode2 class="size-4 shrink-0 text-muted-foreground" />
									<Mono class="min-w-0 flex-1 truncate">{f.f}</Mono>
									<span class="hidden text-[12.5px] text-muted-foreground sm:inline">{f.what}</span>
									{#if f.state === 'differs' && t.changed}<Tag tone="warn">change planned</Tag
										>{:else}<Tag>unchanged</Tag>{/if}
								</li>
							{/each}
						</ul>
					</div>
				</div>
			</Panel>
		{/each}
	</Tabs.Content>

	<Tabs.Content value="middlewares">
		<Panel
			title="30-user-middlewares.yml"
			description="Middlewares you write here can be switched on per service. Invalid YAML keeps the last applied file."
			flush
		>
			{#snippet actions()}<Button size="sm">Validate and plan</Button>{/snippet}
			<div class="grid lg:grid-cols-[minmax(0,1fr)_16rem]">
				<!-- CodeMirror 6 in the real page (10-ui.md §4); static here. -->
				<pre
					class="overflow-x-auto bg-surface-2/40 p-4 font-mono text-[12.5px] leading-relaxed lg:rounded-bl-2xl"><code
						><span class="text-muted-foreground">1</span>  http:
<span class="text-muted-foreground">2</span>    middlewares:
<span class="text-muted-foreground">3</span>      big-uploads:
<span class="text-muted-foreground">4</span>        buffering:
<span class="text-muted-foreground">5</span>          maxRequestBodyBytes: 2147483648
<span class="text-muted-foreground">6</span>      office-only:
<span class="text-muted-foreground">7</span>        ipAllowList:
<span class="text-muted-foreground">8</span>          sourceRange: [192.0.2.0/24]</code
					></pre>
				<div class="border-t p-4 lg:border-t-0 lg:border-l">
					<div class="text-[12px] font-medium">Found 2 middlewares</div>
					<ul class="mt-2 space-y-1.5 text-[13px]">
						<li>
							<Mono>big-uploads</Mono> <span class="text-muted-foreground">· used by Photos</span>
						</li>
						<li><Mono>office-only</Mono> <span class="text-muted-foreground">· unused</span></li>
					</ul>
				</div>
			</div>
		</Panel>
	</Tabs.Content>

	<Tabs.Content value="certs">
		<Panel
			title="Certificates"
			description="Issued by Google Trust Services through DNS-01 answered by Hashi; Traefik hosts never hold the DNS token"
			flush
		>
			<ul class="divide-y text-[13px]">
				{#each [['media.example.com', '18 days', 'warn'], ['grafana.example.com', '41 days', 'neutral'], ['git.example.com', '63 days', 'neutral'], ['hashi.example.com', '77 days', 'neutral']] as [d, e, tone] (d)}
					<li class="flex items-center gap-3 px-4 py-2.5">
						<span class="flex-1">{d}</span><span class="hidden text-muted-foreground sm:inline"
							>both hosts · hashi-gts</span
						><Tag tone={tone === 'warn' ? 'warn' : 'neutral'}>{e}</Tag>
					</li>
				{/each}
			</ul>
		</Panel>
	</Tabs.Content>
</Tabs.Root>
