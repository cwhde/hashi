<script lang="ts">
	import { onMount } from 'svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import * as Select from '$lib/components/ui/select';
	import * as Sheet from '$lib/components/ui/sheet';
	import { Pencil, Plus, Send, Trash2 } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '$lib/api/client';
	import { withReauth } from '$lib/auth/reauth';

	// 07 §6: providers and the default route with severity, cooldown and recovery. Settings holds this setup; the
	// deliveries it produces are on the Status page (RecentNotifications).
	let providers = $state<Schemas['NotificationProvider'][]>([]);
	let route = $state<Schemas['NotificationRoute']>({
		min_severity: 'down',
		cooldown_seconds: 600,
		recovery: true
	});
	let error = $state('');
	let notice = $state('');
	let busy = $state(false);

	async function load() {
		const [p, r] = await Promise.all([
			api.GET('/api/notifications/providers'),
			api.GET('/api/notifications/route')
		]);
		if (!p.data || !r.data) throw p.error ?? r.error;
		providers = p.data.providers;
		route = r.data;
	}
	onMount(() => {
		load().catch((e) => (error = problemText(e)));
	});

	async function act(fn: () => Promise<void>, done = '') {
		error = notice = '';
		busy = true;
		try {
			await fn();
			notice = done;
			await load();
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = false;
		}
	}
	const saveRoute = () =>
		act(async () => {
			const r = await api.PUT('/api/notifications/route', { body: route });
			if (r.error) throw r.error;
		}, 'Route saved.');
	const test = (id: string) =>
		act(async () => {
			const r = await api.POST('/api/notifications/providers/{id}/test', {
				params: { path: { id } }
			});
			if (r.error) throw r.error;
		}, 'Test sent. Its result is on the Status page under Notifications.');
	const remove = (id: string) =>
		act(async () => {
			await withReauth(() =>
				api.DELETE('/api/notifications/providers/{id}', { params: { path: { id } } })
			);
		}, 'Provider removed.');

	// ---- add and edit ----
	const blank = () => ({
		type: 'telegram' as 'telegram' | 'smtp',
		name: '',
		bot_token: '',
		chat_id: '',
		host: '',
		port: '587',
		tls: 'starttls',
		username: '',
		password: '',
		from: '',
		to: ''
	});
	let addOpen = $state(false);
	let editing = $state<string | null>(null);
	let f = $state(blank());
	function openAdd() {
		f = blank();
		editing = null;
		chats = [];
		addError = '';
		addOpen = true;
	}
	function openEdit(p: Schemas['NotificationProvider']) {
		f = {
			...blank(),
			type: p.type === 'smtp' ? 'smtp' : 'telegram',
			name: p.name,
			chat_id: p.chat_id ?? '',
			host: p.host ?? '',
			port: String(p.port ?? 587),
			tls: p.tls ?? 'starttls',
			username: p.username ?? '',
			from: p.from ?? '',
			to: p.to ?? ''
		};
		editing = p.id;
		chats = [];
		addError = '';
		addOpen = true;
	}
	let chats = $state<{ id: string; title: string }[]>([]);
	let addError = $state('');

	async function discover() {
		addError = '';
		busy = true;
		try {
			const r = await api.POST('/api/notifications/telegram/discover', {
				body: { bot_token: f.bot_token }
			});
			if (r.error || !r.data) throw r.error;
			chats = r.data.chats;
			if (chats.length) f.chat_id = chats[0]!.id;
			else addError = 'No messages found. Send the bot any message in Telegram, then try again.';
		} catch (e) {
			addError = problemText(e);
		} finally {
			busy = false;
		}
	}
	async function add() {
		addError = '';
		busy = true;
		try {
			const fields =
				f.type === 'telegram'
					? { name: f.name, bot_token: f.bot_token || undefined, chat_id: f.chat_id }
					: {
							name: f.name,
							host: f.host,
							port: Number(f.port),
							tls: f.tls,
							username: f.username || undefined,
							password: f.password || undefined,
							from: f.from,
							to: f.to
						};
			const id = editing;
			await withReauth(() =>
				id
					? api.PUT('/api/notifications/providers/{id}', {
							params: { path: { id } },
							body: fields
						})
					: api.POST('/api/notifications/providers', { body: { type: f.type, ...fields } })
			);
			f.bot_token = f.password = '';
			addOpen = false;
			await load();
		} catch (e) {
			addError = problemText(e);
		} finally {
			busy = false;
		}
	}
	const providerText = (p: Schemas['NotificationProvider']) =>
		p.type === 'telegram' ? `Telegram · chat ${p.chat_id}` : `Email · ${p.host} to ${p.to}`;
</script>

{#if error}<p role="alert" class="mb-3 text-[13px] text-bad-text">{error}</p>{/if}
{#if notice}<p role="status" class="mb-3 text-[13px] text-ok-text">{notice}</p>{/if}
<div class="grid gap-4 lg:grid-cols-2">
	<Panel
		title="Where alerts go"
		description="The default route. An endpoint's settings can send it elsewhere."
	>
		<div class="space-y-3 text-[13px]">
			<div class="space-y-1.5">
				<Label for="route-provider">Send to</Label>
				<Select.Root
					type="single"
					value={route.provider_id ?? ''}
					onValueChange={(v) => (route.provider_id = v || undefined)}
				>
					<Select.Trigger id="route-provider" class="w-full"
						>{providers.find((p) => p.id === route.provider_id)?.name ?? 'Nobody'}</Select.Trigger
					>
					<Select.Content>
						<Select.Item value="">Nobody</Select.Item>
						{#each providers as p (p.id)}<Select.Item value={p.id}>{p.name}</Select.Item>{/each}
					</Select.Content>
				</Select.Root>
			</div>
			<div class="grid grid-cols-2 gap-3">
				<div class="space-y-1.5">
					<Label for="route-severity">Alert when</Label>
					<Select.Root type="single" bind:value={route.min_severity}>
						<Select.Trigger id="route-severity" class="w-full"
							>{route.min_severity === 'down' ? 'Down' : 'Degraded or down'}</Select.Trigger
						>
						<Select.Content>
							<Select.Item value="down">Down</Select.Item>
							<Select.Item value="degraded">Degraded or down</Select.Item>
						</Select.Content>
					</Select.Root>
				</div>
				<div class="space-y-1.5">
					<Label for="route-cooldown">Cooldown (seconds)</Label>
					<Input id="route-cooldown" type="number" min="0" bind:value={route.cooldown_seconds} />
				</div>
			</div>
			<label class="flex items-center justify-between gap-3">
				<span>Send recovery messages</span><Switch
					bind:checked={route.recovery}
					aria-label="Send recovery messages"
				/>
			</label>
			{#if !route.provider_id}
				<p class="text-warn-text">No alerts are sent until you choose a provider here.</p>
			{/if}
			<div class="flex justify-end">
				<Button size="sm" onclick={saveRoute} disabled={busy}>Save route</Button>
			</div>
		</div>
	</Panel>
	<Panel title="Providers" flush>
		{#snippet actions()}<Button size="sm" onclick={openAdd}><Plus />Add provider</Button>{/snippet}
		{#if providers.length === 0}
			<p class="px-4 py-3 text-[13px] text-muted-foreground">
				Add Telegram or email to receive alerts.
			</p>
		{/if}
		<ul class="divide-y">
			{#each providers as p (p.id)}
				<li class="flex items-center gap-2 px-4 py-2.5 text-[13px]">
					<span class="min-w-0 flex-1"
						><span class="block truncate font-medium">{p.name}</span><span
							class="block truncate text-[12px] text-muted-foreground">{providerText(p)}</span
						></span
					>
					<Button variant="outline" size="sm" onclick={() => test(p.id)} disabled={busy}
						><Send />Send test</Button
					>
					<Button
						variant="ghost"
						size="icon-sm"
						aria-label="Edit {p.name}"
						onclick={() => openEdit(p)}
						disabled={busy}><Pencil /></Button
					>
					<Button
						variant="ghost"
						size="icon-sm"
						aria-label="Remove {p.name}"
						onclick={() => remove(p.id)}
						disabled={busy}><Trash2 /></Button
					>
				</li>
			{/each}
		</ul>
	</Panel>
</div>

<Sheet.Root bind:open={addOpen}>
	<Sheet.Content class="w-full sm:max-w-md">
		<Sheet.Header>
			<Sheet.Title>{editing ? 'Edit provider' : 'Add provider'}</Sheet.Title>
			<Sheet.Description
				>{editing
					? 'Leave the secret empty to keep the stored one.'
					: 'Its secret is stored encrypted and never shown again.'}</Sheet.Description
			>
		</Sheet.Header>
		<div class="space-y-4 overflow-y-auto px-4">
			<div class="space-y-1.5">
				<Label for="np-type">Type</Label>
				<Select.Root type="single" bind:value={f.type} disabled={!!editing}>
					<Select.Trigger id="np-type" class="w-full"
						>{f.type === 'telegram' ? 'Telegram' : 'Email (SMTP)'}</Select.Trigger
					>
					<Select.Content>
						<Select.Item value="telegram">Telegram</Select.Item>
						<Select.Item value="smtp">Email (SMTP)</Select.Item>
					</Select.Content>
				</Select.Root>
			</div>
			<div class="space-y-1.5">
				<Label for="np-name">Name</Label><Input id="np-name" bind:value={f.name} />
			</div>
			{#if f.type === 'telegram'}
				<div class="space-y-1.5">
					<Label for="np-token">Bot token</Label><Input
						id="np-token"
						type="password"
						autocomplete="off"
						bind:value={f.bot_token}
					/>
				</div>
				<div class="space-y-1.5">
					<Label for="np-chat">Chat</Label>
					<div class="flex gap-2">
						<Input id="np-chat" bind:value={f.chat_id} placeholder="Number or @channel" />
						<Button variant="outline" onclick={discover} disabled={busy || !f.bot_token}
							>Find chat</Button
						>
					</div>
					<p class="text-[12px] text-muted-foreground">
						{#if chats.length}Found: {chats.map((c) => c.title).join(', ')}.{:else}Send the bot a
							message in Telegram, then find it here.{/if}
					</p>
				</div>
			{:else}
				<div class="grid grid-cols-[1fr_6rem] gap-3">
					<div class="space-y-1.5">
						<Label for="np-host">Server</Label><Input id="np-host" bind:value={f.host} />
					</div>
					<div class="space-y-1.5">
						<Label for="np-port">Port</Label><Input
							id="np-port"
							bind:value={f.port}
							inputmode="numeric"
						/>
					</div>
				</div>
				<div class="space-y-1.5">
					<Label for="np-tls">Encryption</Label>
					<Select.Root type="single" bind:value={f.tls}>
						<Select.Trigger id="np-tls" class="w-full"
							>{f.tls === 'starttls' ? 'STARTTLS' : 'TLS from the start'}</Select.Trigger
						>
						<Select.Content>
							<Select.Item value="starttls">STARTTLS</Select.Item>
							<Select.Item value="tls">TLS from the start</Select.Item>
						</Select.Content>
					</Select.Root>
				</div>
				<div class="space-y-1.5">
					<Label for="np-user">Username</Label><Input
						id="np-user"
						autocomplete="off"
						bind:value={f.username}
					/>
				</div>
				<div class="space-y-1.5">
					<Label for="np-pass">Password</Label><Input
						id="np-pass"
						type="password"
						autocomplete="off"
						bind:value={f.password}
					/>
				</div>
				<div class="space-y-1.5">
					<Label for="np-from">From</Label><Input id="np-from" bind:value={f.from} />
				</div>
				<div class="space-y-1.5">
					<Label for="np-to">To</Label><Input id="np-to" bind:value={f.to} />
				</div>
			{/if}
			{#if addError}<p role="alert" class="text-[13px] text-bad-text">{addError}</p>{/if}
		</div>
		<Sheet.Footer class="flex-row justify-end gap-2 border-t">
			<Button variant="outline" onclick={() => (addOpen = false)}>Cancel</Button>
			<Button onclick={add} disabled={busy || !f.name.trim()}
				>{editing ? 'Save changes' : 'Add provider'}</Button
			>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>
