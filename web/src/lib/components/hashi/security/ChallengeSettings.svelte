<script lang="ts">
	import { Skeleton } from '#lib/components/ui/skeleton/index.ts';
	import MoreOptions from '#lib/components/hashi/MoreOptions.svelte';
	import { Draft, guardUnsaved } from '#lib/unsaved.svelte.ts';
	import { onMount } from 'svelte';
	import Panel from '#lib/components/hashi/Panel.svelte';
	import HelpTip from '#lib/components/hashi/HelpTip.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import { Switch } from '#lib/components/ui/switch/index.ts';
	import * as Select from '#lib/components/ui/select/index.ts';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { withReauth } from '#lib/auth/reauth.ts';

	// CAPTCHA (06 §11): Cap runs next to Hashi; Hashi can create its own key there. Saving creates or moves the
	// challenge service, whose plans appear under Pending changes.
	let form = $state<Schemas['CaptchaSettingsInput'] | null>(null);
	let info = $state<Schemas['CaptchaSettings'] | null>(null);
	let services = $state<Schemas['Service'][]>([]);
	let note = $state('');
	let error = $state('');
	let busy = $state(false);
	const draft = new Draft(() => form);
	// A suggested challenge page is not saved yet, so Save stays on, but only the owner's own edits ask before leaving.
	const shown = new Draft(() => form);
	guardUnsaved(() => shown.dirty);

	function take(c: Schemas['CaptchaSettings']) {
		info = c;
		form = {
			enabled: c.enabled,
			challenge_domain: c.challenge_domain,
			cap_address: c.cap_address,
			cap_port: c.cap_port,
			verify_url: c.verify_url,
			site_key: c.site_key,
			secret_key: '',
			timeout_seconds: c.timeout_seconds,
			admin_service_id: c.admin_service_id ?? ''
		};
		draft.mark();
		shown.mark();
	}

	onMount(async () => {
		const [c, sv, g] = await Promise.all([
			api.GET('/api/security/captcha/settings'),
			api.GET('/api/services'),
			api.GET('/api/settings/general')
		]);
		if (c.error || !c.data) {
			error = problemText(c.error);
			return;
		}
		take(c.data);
		if (form && !form.challenge_domain && g.data?.root_domain)
			form.challenge_domain = `captcha.hashi.${g.data.root_domain}`;
		shown.mark();
		services = (sv.data?.services ?? []).filter((s) => s.origin === 'user');
	});

	async function run(what: () => Promise<void>) {
		busy = true;
		error = note = '';
		try {
			await what();
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = false;
		}
	}
	const createKey = () =>
		run(async () => {
			take(await withReauth(() => api.POST('/api/security/captcha/key')));
			note = 'Cap created a site key for Hashi; its secret is in the vault.';
		});
	const save = () =>
		run(async () => {
			if (!form) return;
			const body = { ...form, admin_service_id: form.admin_service_id || undefined };
			const res = await withReauth(() => api.PUT('/api/security/captcha/settings', { body }));
			take(res.settings);
			const waiting = res.plans.filter((p) => p.status === 'pending').length;
			note = waiting
				? `Saved. ${waiting} change${waiting === 1 ? '' : 's'} wait under Pending changes.`
				: 'Saved.';
		});
	const test = () =>
		run(async () => {
			const r = await api.POST('/api/security/captcha/test');
			if (r.error || !r.data) throw r.error;
			if (r.data.ok) note = r.data.message;
			else error = r.data.message;
		});
	const dashboardName = $derived(
		services.find((s) => s.id === form?.admin_service_id)?.host_name ?? 'None'
	);
</script>

{#if form && info}
	<div class="min-w-0">
		<Panel title="Cap challenge">
			{#snippet help()}<HelpTip topic="the challenge"
					><p>
						Too many requests from one address send its browser to the challenge page; ignoring it
						leads to a ban.
					</p>
					<p>Solving ends the challenge but leaves the counters alone.</p>
					<p>Signed-in users and addresses in the firewall allow list never see it.</p></HelpTip
				>{/snippet}
			<div class="space-y-3">
				<div class="flex items-center justify-between gap-3">
					<Label for="cc-on">Challenge visitors with a CAPTCHA</Label><Switch
						id="cc-on"
						bind:checked={form.enabled}
					/>
				</div>
				<div class="space-y-1.5">
					<Label for="cc-domain">Challenge page</Label>
					<Input id="cc-domain" bind:value={form.challenge_domain} />
				</div>
				<div class="space-y-1.5">
					<Label for="cc-dash">Cap dashboard service</Label>
					<Select.Root type="single" bind:value={form.admin_service_id}>
						<Select.Trigger id="cc-dash" class="w-full">{dashboardName}</Select.Trigger>
						<Select.Content>
							<Select.Item value="">None</Select.Item>
							{#each services as s (s.id)}<Select.Item value={s.id}
									>{s.host_name || s.name}</Select.Item
								>{/each}
						</Select.Content>
					</Select.Root>
				</div>
				<div class="space-y-1.5">
					<Label for="cc-key">Site key</Label>
					<div class="flex gap-2">
						<Input id="cc-key" bind:value={form.site_key} class="font-mono" />
						{#if info.can_create_key}<Button
								variant="outline"
								class="shrink-0"
								disabled={busy}
								onclick={createKey}>Create key in Cap</Button
							>{/if}
					</div>
				</div>
				<div class="space-y-1.5">
					<Label for="cc-secret">Secret key</Label>
					<Input
						id="cc-secret"
						type="password"
						bind:value={form.secret_key}
						placeholder={info.has_secret ? 'Stored in the vault' : 'sk-…'}
						class="h-10"
					/>
				</div>
				<MoreOptions label="Where Hashi and the edges reach Cap">
					<div class="grid gap-3 sm:grid-cols-[1fr_6rem]">
						<div class="space-y-1.5">
							<Label for="cc-addr">Address for the edges</Label>
							<Input id="cc-addr" bind:value={form.cap_address} class="font-mono" />
						</div>
						<div class="space-y-1.5">
							<Label for="cc-port">Port</Label>
							<Input id="cc-port" type="number" bind:value={form.cap_port} />
						</div>
						<div class="space-y-1.5 sm:col-span-2">
							<Label for="cc-verify">Address for Hashi</Label>
							<Input id="cc-verify" bind:value={form.verify_url} class="font-mono" />
						</div>
					</div>
				</MoreOptions>
				{#if error}<p role="alert" class="text-md text-bad-text">{error}</p>{/if}
				{#if note}<p role="status" class="text-md">{note}</p>{/if}
				<div class="flex flex-wrap justify-end gap-2 pt-1">
					<Button size="sm" variant="outline" disabled={busy} onclick={test}
						>Test verification</Button
					><Button size="sm" disabled={busy || !draft.dirty} onclick={save}>Save</Button>
				</div>
			</div>
		</Panel>
	</div>
{:else if error}
	<p role="alert" class="text-md text-bad-text">{error}</p>
{:else}<Skeleton class="h-40 rounded-2xl" />
{/if}
