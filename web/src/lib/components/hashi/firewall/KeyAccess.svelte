<script lang="ts" module>
	export type Access = {
		mode: 'install' | 'manual';
		kind: 'password' | 'private_key';
		credential: string;
		passphrase: string;
	};
	export const newAccess = (): Access => ({
		mode: 'install',
		kind: 'password',
		credential: '',
		passphrase: ''
	});
	/** The one-time credential of a key install. */
	export const accessBody = (a: Access) =>
		a.kind === 'password'
			? { password: a.credential } // hashi:allow-secret-pattern
			: { private_key: a.credential, passphrase: a.passphrase || undefined };
</script>

<script lang="ts">
	// How Hashi's key gets onto a host: installed by Hashi with the admin's permission, or pasted by hand.
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import { Check, Copy } from '@lucide/svelte';

	let {
		access = $bindable(),
		publicKey,
		user
	}: { access: Access; publicKey: string; user: string } = $props();
	let copied = $state(false);
</script>

<fieldset class="space-y-3 rounded-xl border p-3 md:col-span-2">
	<legend class="px-1 text-[13px] font-medium">Hashi's key on this host</legend>
	<label class="flex items-start gap-2.5 text-[13px]">
		<input type="radio" bind:group={access.mode} value="install" class="mt-0.5 accent-primary" />
		<span
			>Let Hashi install its key. Hashi logs in once as {user || 'this user'} with what you enter here,
			adds one line to <Mono>authorized_keys</Mono>, and forgets what you entered.</span
		>
	</label>
	<label class="flex items-start gap-2.5 text-[13px]">
		<input type="radio" bind:group={access.mode} value="manual" class="mt-0.5 accent-primary" />
		<span>I'll do it myself</span>
	</label>
	{#if access.mode === 'install'}
		<div class="grid gap-3 md:grid-cols-2">
			<div class="space-y-1.5">
				<Label for="ka-kind">Log in once with</Label>
				<select
					id="ka-kind"
					bind:value={access.kind}
					class="h-10 w-full rounded-[10px] border bg-transparent px-3 text-[13px]"
				>
					<option value="password">A password</option>
					<option value="private_key">A private key</option>
				</select>
			</div>
			{#if access.kind === 'password'}
				<div class="space-y-1.5">
					<Label for="ka-pw">Password for {user || 'this user'}</Label>
					<Input
						id="ka-pw"
						type="password"
						bind:value={access.credential}
						autocomplete="off"
						class="h-10"
					/>
				</div>
			{:else}
				<div class="space-y-1.5">
					<Label for="ka-pass">Passphrase (if any)</Label>
					<Input
						id="ka-pass"
						type="password"
						bind:value={access.passphrase}
						autocomplete="off"
						class="h-10"
					/>
				</div>
				<div class="space-y-1.5 md:col-span-2">
					<Label for="ka-key">Private key</Label>
					<textarea
						id="ka-key"
						bind:value={access.credential}
						rows="4"
						autocomplete="off"
						class="w-full rounded-[10px] border bg-transparent px-3 py-2 font-mono text-[12.5px]"
					></textarea>
				</div>
			{/if}
		</div>
	{:else}
		<div>
			<p class="text-[12.5px] text-muted-foreground">
				Add this line to <Mono>~/.ssh/authorized_keys</Mono> of {user || 'this user'} on the host, then
				continue.
			</p>
			<div class="mt-2 flex items-start gap-2">
				<Mono class="block flex-1 rounded-lg bg-surface-2 px-3 py-2 break-all"
					><span data-testid="hashi-public-key">{publicKey || '…'}</span></Mono
				>
				<Button
					type="button"
					variant="outline"
					size="sm"
					disabled={!publicKey}
					onclick={async () => {
						await navigator.clipboard.writeText(publicKey);
						copied = true;
					}}
				>
					{#if copied}<Check />Copied{:else}<Copy />Copy{/if}
				</Button>
			</div>
		</div>
	{/if}
</fieldset>
