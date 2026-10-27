<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import Mono from '$lib/components/hashi/Mono.svelte';
	import { cn } from '$lib/utils';
	import { Check, CircleAlert, ShieldCheck } from '@lucide/svelte';

	// 02-setup-auth.md §1: a resumable, persisted sequence; a step completes only after its validation succeeded.
	const steps = [
		{ t: 'Bootstrap sign-in', s: 'done' },
		{ t: 'Base settings', s: 'done' },
		{ t: 'DNS provider', s: 'done' },
		{ t: 'Certificates', s: 'done' },
		{ t: 'Traefik host', s: 'current' },
		{ t: 'Firewall host', s: 'todo' },
		{ t: 'Hashi itself', s: 'todo' },
		{ t: 'HTTPS check', s: 'todo' },
		{ t: 'Passkey and recovery key', s: 'todo' },
		{ t: 'Optional integrations', s: 'todo' },
		{ t: 'Finish', s: 'todo' }
	];
	let fingerprintShown = $state(true);
</script>

<svelte:head><title>Set up Hashi</title></svelte:head>
<div
	class="mx-auto grid min-h-dvh max-w-6xl gap-8 px-4 py-8 md:grid-cols-[16rem_minmax(0,1fr)] md:py-14"
>
	<aside>
		<div class="mb-6 flex items-center gap-2.5">
			<span
				class="grid size-8 place-items-center rounded-lg bg-primary text-[16px] font-semibold text-primary-foreground"
				>橋</span
			><span class="text-[15px] font-semibold">Set up Hashi</span>
		</div>
		<ol class="space-y-0.5">
			{#each steps as s, i (s.t)}
				<li
					class={cn(
						'flex h-9 items-center gap-3 rounded-[10px] px-2.5 text-[13.5px]',
						s.s === 'current' && 'bg-surface-2 font-medium',
						s.s === 'todo' && 'text-muted-foreground'
					)}
				>
					<span
						class={cn(
							'tnum grid size-5 shrink-0 place-items-center rounded-full border text-[11px]',
							s.s === 'done' && 'border-ok/40 bg-ok/15 text-ok-text',
							s.s === 'current' && 'border-primary text-foreground'
						)}
					>
						{#if s.s === 'done'}<Check class="size-3" />{:else}{i + 1}{/if}
					</span>
					{s.t}
				</li>
			{/each}
		</ol>
		<p class="mt-6 text-[12.5px] text-muted-foreground">
			Progress is saved after every step. You can close this page and continue later.
		</p>
	</aside>

	<main class="min-w-0">
		<p class="text-[13px] text-muted-foreground">Step 5 of 11</p>
		<h1 class="mt-1 text-[24px] font-semibold">Connect a Traefik host</h1>
		<p class="mt-1 max-w-[62ch] text-[13.5px] text-muted-foreground">
			Hashi installs and runs its own Traefik on this machine over SSH. Existing Traefik config is
			backed up and only replaced after you confirm.
		</p>

		<section class="mt-6 rounded-3xl border bg-card p-6">
			<div class="grid gap-4 md:grid-cols-2">
				<div class="space-y-1.5">
					<Label for="h">Host or IP</Label><Input id="h" value="10.20.4.11" class="h-10" />
				</div>
				<div class="space-y-1.5">
					<Label for="p">SSH port</Label><Input id="p" value="22" class="h-10" />
				</div>
				<div class="space-y-1.5">
					<Label for="u">User</Label><Input id="u" value="root" class="h-10" />
				</div>
				<div class="space-y-1.5">
					<Label for="k">Private key</Label><Input
						id="k"
						value="•••••••••• (pasted, stored in the vault)"
						class="h-10"
						disabled
					/>
				</div>
			</div>
			{#if fingerprintShown}
				<div class="mt-5 rounded-2xl border border-primary/40 bg-primary/[0.06] p-4">
					<div class="flex items-center gap-2 text-[13.5px] font-medium">
						<ShieldCheck class="size-4" />Confirm this host key
					</div>
					<p class="mt-1 text-[13px] text-muted-foreground">
						First contact with 10.20.4.11. Compare it with <Mono
							>ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub</Mono
						> on the host.
					</p>
					<Mono class="mt-3 block rounded-lg bg-surface-2 px-3 py-2"
						>ED25519 SHA256:3k9rQv0w8V5nX1bq0yT7mJ2cL4pZx6hN8uE1sA0fGkI</Mono
					>
					<div class="mt-3 flex gap-2">
						<Button size="sm">It matches, pin it</Button><Button
							size="sm"
							variant="ghost"
							onclick={() => (fingerprintShown = false)}>It doesn't match</Button
						>
					</div>
				</div>
			{:else}
				<p
					class="mt-5 flex gap-2 rounded-2xl border border-bad/30 bg-bad/10 p-4 text-[13px] text-bad-text"
				>
					<CircleAlert class="mt-0.5 size-4 shrink-0" />Connection stopped. A different key can mean
					the wrong machine or an interception. Check the address and try again.
				</p>
			{/if}
			<ul class="mt-5 space-y-1.5 text-[13px]">
				<li class="flex items-center gap-2">
					<Check class="size-4 text-ok-text" />SSH login as root
				</li>
				<li class="flex items-center gap-2">
					<Check class="size-4 text-ok-text" />Alpine 3.24, apk
				</li>
				<li class="flex items-center gap-2">
					<span class="size-4 text-center text-muted-foreground">·</span><span
						class="text-muted-foreground"
						>Existing Traefik and config discovery — after the key is pinned</span
					>
				</li>
			</ul>
		</section>
		<div class="mt-6 flex justify-between">
			<Button variant="ghost">Back</Button><Button disabled>Continue</Button>
		</div>
	</main>
</div>
