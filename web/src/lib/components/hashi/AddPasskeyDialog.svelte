<script lang="ts">
	import * as Dialog from '#lib/components/ui/dialog/index.ts';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import { Checkbox } from '#lib/components/ui/checkbox/index.ts';
	import { Check, Fingerprint, KeyRound, Usb } from '@lucide/svelte';
	import { api, problemText, type Schemas } from '#lib/api/client.ts';
	import { reauthenticate, withReauth } from '#lib/auth/reauth.ts';
	import { session } from '#lib/auth/session.svelte.ts';
	import { createPasskey, usePasskey } from '#lib/auth/webauthn.ts';
	import { vaultState } from '#lib/vault.svelte.ts';
	import { cn } from '#lib/utils.ts';

	// Adding a passkey, as a sequence the admin can follow (bug 37). Creating one is two browser prompts that look
	// alike: first "confirm it's you" with a passkey Hashi already knows, then "create" on the new device. The vault
	// link is a third prompt to the same new device. Each step names the device it wants.
	//   for="signin": add a sign-in passkey, optionally also let it unlock the vault.
	//   for="vault":  pick an existing passkey to unlock the vault, or create a new one for it.
	let {
		open = $bindable(false),
		for: purpose,
		passkeys,
		onchanged
	}: {
		open?: boolean;
		for: 'signin' | 'vault';
		passkeys: { id: string; name: string }[];
		onchanged: () => Promise<void>;
	} = $props();

	type Step = 'choose' | 'confirm' | 'create' | 'link' | 'done';
	let step = $state<Step>('choose');
	let busy = $state(false);
	let error = $state('');
	let name = $state('');
	let choice = $state(''); // vault: an existing passkey id, or 'new'
	let alsoVault = $state(true);
	let created = $state<{ id: string; name: string } | null>(null);
	let linked = $state('');

	const vault = $derived(vaultState.current);
	const unlocking = $derived(new Set((vault?.unlockers ?? []).map((u) => u.passkey_id)));
	const linkable = $derived(passkeys.filter((p) => !unlocking.has(p.id)));
	const known = $derived(passkeys.map((p) => p.name).join(', '));
	const wantsVault = $derived(purpose === 'vault' || (alsoVault && !!vault && !vault.locked));
	const creating = $derived(purpose === 'signin' || choice === 'new');

	// The sequence for this run; the rail shows all of it from the start.
	const steps = $derived<Exclude<Step, 'choose' | 'done'>[]>(
		creating ? (wantsVault ? ['confirm', 'create', 'link'] : ['confirm', 'create']) : ['link']
	);
	const position = (s: Step) => (s === 'done' ? steps.length : steps.indexOf(s as never));
	const phase = (s: Step) =>
		position(step) > position(s) ? 'done' : step === s ? 'current' : 'next';

	function confirmedRecently() {
		const until = session.current?.reauth_valid_until;
		return !!until && new Date(until).getTime() - Date.now() > 60_000;
	}

	$effect(() => {
		if (!open) return;
		// Reset on every open.
		step = purpose === 'vault' ? 'choose' : 'confirm';
		error = '';
		name = '';
		choice = '';
		alsoVault = true;
		created = null;
		linked = '';
	});

	function begin() {
		error = '';
		if (!creating) return void (step = 'link');
		step = confirmedRecently() ? 'create' : 'confirm';
	}

	function message(e: unknown, at: Step): string {
		if (e && typeof e === 'object' && 'type' in e) return problemText(e);
		const dom = e instanceof DOMException ? e.name : '';
		if (at === 'confirm' && (dom === 'NotAllowedError' || dom === 'AbortError'))
			return `No known passkey answered. Use one Hashi already has (${known}), not the new device: it holds nothing for Hashi yet.`;
		if (at === 'create' && dom === 'InvalidStateError')
			return 'This device already holds a passkey for Hashi. Pick it in the vault list instead, or use another device.';
		if (at === 'create' && (dom === 'NotAllowedError' || dom === 'AbortError'))
			return 'The prompt was closed before a passkey was created. Nothing was saved; try again.';
		if (at === 'link' && (dom === 'NotAllowedError' || dom === 'AbortError'))
			return `The prompt was closed, or a different device answered. Use ${linkName()}.`;
		return e instanceof Error ? e.message : 'That step failed. Nothing was changed.';
	}

	async function act(at: Step, f: () => Promise<void>) {
		busy = true;
		error = '';
		try {
			await f();
		} catch (e) {
			error = message(e, at);
		} finally {
			busy = false;
		}
	}

	const confirm = () =>
		act('confirm', async () => {
			await reauthenticate();
			step = 'create';
		});

	const create = () =>
		act('create', async () => {
			const opts = await withReauth(() => api.POST('/api/auth/passkeys/register/options'));
			const credential = await createPasskey(opts.options);
			const pk: Schemas['Passkey'] = await withReauth(() =>
				api.POST('/api/auth/passkeys/register/verify', {
					body: {
						ceremony_id: opts.ceremony_id,
						name: name.trim() || 'Security key',
						credential
					}
				})
			);
			created = { id: pk.id, name: pk.name };
			await onchanged();
			step = wantsVault ? 'link' : 'done';
		});

	const linkId = () => (creating ? created?.id : choice) ?? '';
	const linkName = () =>
		(creating ? created?.name : passkeys.find((p) => p.id === choice)?.name) ?? 'that passkey';

	const link = () =>
		act('link', async () => {
			const opts = await api.POST('/api/vault/unlockers/options', {
				body: { passkey_id: linkId() }
			});
			if (opts.error || !opts.data) throw opts.error;
			const credential = await usePasskey(opts.data.options);
			const res = await api.POST('/api/vault/unlockers', {
				body: { ceremony_id: opts.data.ceremony_id, credential }
			});
			if (res.error || !res.data) throw res.error;
			vaultState.set(res.data);
			linked = linkName();
			step = 'done';
		});

	const copy: Record<Exclude<Step, 'choose' | 'done'>, { title: string; body: () => string }> = {
		confirm: {
			title: 'Confirm it’s you',
			body: () =>
				`Use a passkey Hashi already knows: ${known}. Not the new device yet, it holds nothing for Hashi.`
		},
		create: {
			title: 'Create the passkey on the new device',
			body: () =>
				'Your browser or password manager asks where to save it. For a USB security key pick “security key” or “another device”, then touch the key and enter its PIN.'
		},
		link: {
			title: 'Let it unlock the vault',
			body: () =>
				`Touch ${linkName()} once more. Hashi asks it for a secret only that device can make (PRF) and wraps the vault key with it.`
		}
	};
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="sm:max-w-lg">
		<Dialog.Header>
			<Dialog.Title
				>{purpose === 'vault'
					? 'Add a passkey that unlocks the vault'
					: 'Add a passkey'}</Dialog.Title
			>
		</Dialog.Header>

		{#if step === 'choose'}
			<fieldset class="space-y-1.5 text-md">
				<legend class="sr-only">Passkey</legend>
				{#each linkable as p (p.id)}
					<label
						class={cn(
							'flex cursor-pointer items-center gap-3 rounded-[10px] border px-3 py-2.5',
							choice === p.id && 'border-primary bg-surface-2'
						)}
					>
						<input type="radio" bind:group={choice} value={p.id} class="accent-primary" />
						<KeyRound class="size-4 text-muted-foreground" />
						<span class="flex-1">{p.name}</span>
					</label>
				{/each}
				<label
					class={cn(
						'flex cursor-pointer items-center gap-3 rounded-[10px] border px-3 py-2.5',
						choice === 'new' && 'border-primary bg-surface-2'
					)}
				>
					<input type="radio" bind:group={choice} value="new" class="accent-primary" />
					<Usb class="size-4 text-muted-foreground" />
					<span class="flex-1">A new passkey</span>
				</label>
			</fieldset>
			{#if purpose === 'vault'}
				<p class="text-xs text-muted-foreground">
					Passkeys kept in a password manager often can’t unlock the vault.
				</p>
			{/if}
			<Dialog.Footer>
				<Button variant="ghost" onclick={() => (open = false)}>Cancel</Button>
				<Button disabled={!choice} onclick={begin}>Continue</Button>
			</Dialog.Footer>
		{:else}
			{#if creating && !created && step !== 'done'}
				<div class="grid gap-3 sm:grid-cols-[1fr_auto] sm:items-end">
					<div class="space-y-1.5">
						<Label for="new-passkey-name">Name</Label>
						<Input
							id="new-passkey-name"
							bind:value={name}
							maxlength={64}
							placeholder="Security key"
							disabled={step !== 'confirm' && step !== 'create'}
						/>
					</div>
					{#if purpose === 'signin' && vault && !vault.locked}
						<label class="flex h-9 items-center gap-2 text-md">
							<Checkbox bind:checked={alsoVault} disabled={busy || step === 'link'} />
							Also unlock the vault with it
						</label>
					{/if}
				</div>
			{/if}

			<ol class="relative space-y-0 text-md">
				{#each steps as s, i (s)}
					{@const st = phase(s)}
					<li class="relative flex gap-3 pb-5 last:pb-0">
						{#if i < steps.length - 1}
							<span
								aria-hidden="true"
								class={cn(
									'absolute top-7 bottom-0 left-[13px] w-px',
									st === 'done' ? 'bg-ok' : 'bg-border'
								)}
							></span>
						{/if}
						<span
							aria-hidden="true"
							class={cn(
								'relative z-10 flex size-[27px] shrink-0 items-center justify-center rounded-full border text-xs font-medium tabular-nums',
								st === 'done' && 'border-ok bg-ok text-primary-foreground',
								st === 'current' && 'border-primary bg-primary text-primary-foreground',
								st === 'next' && 'bg-surface text-muted-foreground'
							)}
						>
							{#if st === 'done'}<Check class="size-3.5" />{:else}{i + 1}{/if}
						</span>
						<div class={cn('min-w-0 flex-1 space-y-2 pt-1', st === 'next' && 'opacity-60')}>
							<p class="font-medium">
								{copy[s].title}<span class="sr-only"
									>{' '}
									({st === 'done' ? 'done' : st === 'current' ? 'current step' : 'next'})</span
								>
							</p>
							{#if st !== 'done'}
								<p class="text-muted-foreground">{copy[s].body()}</p>
							{:else if s === 'confirm'}
								<p class="text-muted-foreground">Confirmed.</p>
							{:else if s === 'create' && created}
								<p class="text-muted-foreground">
									“{created.name}” created
								</p>
							{/if}
							{#if st === 'current'}
								<Button
									size="sm"
									disabled={busy}
									onclick={s === 'confirm' ? confirm : s === 'create' ? create : link}
								>
									<Fingerprint />{busy
										? 'Waiting for the device…'
										: s === 'confirm'
											? 'Confirm with a known passkey'
											: s === 'create'
												? 'Create passkey'
												: `Link ${linkName()}`}
								</Button>
								{#if error}<p role="alert" class="text-bad-text">{error}</p>{/if}
							{/if}
						</div>
					</li>
				{/each}
			</ol>

			{#if step === 'done'}
				<p role="status" class="rounded-[10px] bg-surface-2 px-3 py-2.5 text-md text-ok-text">
					{#if linked}{linked} can unlock the vault now.{:else}Passkey added. You can sign in with “{created?.name}”.{/if}
				</p>
			{/if}
			<Dialog.Footer>
				{#if step === 'done'}
					<Button onclick={() => (open = false)}>Done</Button>
				{:else}
					<Button variant="ghost" onclick={() => (open = false)}
						>{created ? 'Close (the passkey stays added)' : 'Cancel'}</Button
					>
				{/if}
			</Dialog.Footer>
		{/if}
	</Dialog.Content>
</Dialog.Root>
