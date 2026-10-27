<script lang="ts">
	import PageHeader from '$lib/components/hashi/PageHeader.svelte';
	import Panel from '$lib/components/hashi/Panel.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Skeleton } from '$lib/components/ui/skeleton';
	import * as Dialog from '$lib/components/ui/dialog';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { Lock, Fingerprint, Plus, CircleAlert, Check } from '@lucide/svelte';
	let reauth = $state(false);
	let saved = $state(false);
</script>

<PageHeader
	title="States"
	description="Patterns every page uses for locks, empty lists, loading, errors and confirmations."
/>

<div
	class="mb-5 flex flex-wrap items-center gap-3 rounded-2xl border border-warn/30 bg-warn/10 px-4 py-3 text-[13px]"
>
	<Lock class="size-4 text-warn-text" />
	<span class="flex-1"
		><span class="font-medium">Vault locked.</span> Syncs and scripts are paused; public pages keep showing
		the last known state. Shown on every page while locked.</span
	>
	<Button size="sm"><Fingerprint />Unlock</Button>
</div>

<div class="grid gap-4 lg:grid-cols-2">
	<Panel title="Empty">
		<div class="py-8 text-center">
			<p class="text-[14px] font-medium">No scripts yet</p>
			<p class="mx-auto mt-1 max-w-[42ch] text-[13px] text-muted-foreground">
				Scripts run as root on your firewall hosts, on a schedule or by hand.
			</p>
			<Button size="sm" class="mt-4"><Plus />New script</Button>
		</div>
	</Panel>
	<Panel title="Loading">
		<div class="space-y-3">
			{#each [0, 1, 2, 3] as i (i)}<div class="flex items-center gap-3">
					<Skeleton class="size-2 rounded-full" /><Skeleton class="h-4 flex-1" /><Skeleton
						class="h-4 w-20"
					/>
				</div>{/each}
		</div>
		<p class="mt-3 text-[12.5px] text-muted-foreground">
			Skeletons keep the final layout; never a page-wide spinner.
		</p>
	</Panel>
	<Panel title="Error">
		<p class="flex gap-2 rounded-xl border border-bad/30 bg-bad/10 p-3 text-[13px] text-bad-text">
			<CircleAlert class="mt-0.5 size-4 shrink-0" /><span
				><span class="font-medium">Couldn't read DNS records.</span> Hetzner answered 401: the token was
				revoked or expired. Replace it in Connections → Hetzner Cloud. Nothing was changed.</span
			>
		</p>
	</Panel>
	<Panel title="Confirmations">
		<div class="flex flex-wrap gap-2">
			<Button variant="outline" size="sm" onclick={() => (reauth = true)}>Passkey check</Button>
			<AlertDialog.Root>
				<AlertDialog.Trigger
					>{#snippet child({ props })}<Button {...props} variant="destructive" size="sm"
							>Delete resource…</Button
						>{/snippet}</AlertDialog.Trigger
				>
				<AlertDialog.Content>
					<AlertDialog.Header
						><AlertDialog.Title>Delete Photos?</AlertDialog.Title><AlertDialog.Description
							>This plans removal of its DNS record, Traefik routes and monitor. You'll review the
							plan before anything changes.</AlertDialog.Description
						></AlertDialog.Header
					>
					<AlertDialog.Footer
						><AlertDialog.Cancel>Keep it</AlertDialog.Cancel><AlertDialog.Action
							>Plan deletion</AlertDialog.Action
						></AlertDialog.Footer
					>
				</AlertDialog.Content>
			</AlertDialog.Root>
			<Button size="sm" onclick={() => (saved = !saved)}
				>{#if saved}<Check />Saved{:else}Save{/if}</Button
			>
		</div>
		<p class="mt-3 text-[12.5px] text-muted-foreground">
			Buttons say what happens. A remote change shows "Applying…" until the API confirms; never
			"done" before.
		</p>
	</Panel>
</div>

<Dialog.Root bind:open={reauth}>
	<Dialog.Content class="sm:max-w-sm">
		<Dialog.Header
			><Dialog.Title>Confirm with your passkey</Dialog.Title><Dialog.Description
				>Approving a firewall change needs a passkey check from the last 5 minutes.</Dialog.Description
			></Dialog.Header
		>
		<Button class="h-10" onclick={() => (reauth = false)}><Fingerprint />Use passkey</Button>
	</Dialog.Content>
</Dialog.Root>
