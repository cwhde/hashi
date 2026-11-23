<script lang="ts">
	import * as Dialog from '#lib/components/ui/dialog/index.ts';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Input } from '#lib/components/ui/input/index.ts';
	import { Label } from '#lib/components/ui/label/index.ts';
	import { KeyRound } from '@lucide/svelte';
	import { api, problemText } from '#lib/api/client.ts';
	import { withReauth } from '#lib/auth/reauth.ts';

	// Edits one address's ban and challenge counters (06 §16). `address` opens the dialog; a /64 row stands for its
	// network, and its first address reaches the same counter.
	let {
		address = $bindable(null),
		ban = 0,
		challenge = 0,
		onsaved
	}: {
		address?: string | null;
		ban?: number;
		challenge?: number;
		onsaved?: () => void | Promise<void>;
	} = $props();

	let editBan = $state(0);
	let editChallenge = $state(0);
	let busy = $state(false);
	let error = $state('');
	// Kept while the dialog closes, so its title doesn't lose the address mid-animation.
	let shown = $state('');
	$effect(() => {
		if (address) {
			shown = address;
			editBan = ban;
			editChallenge = challenge;
			error = '';
		}
	});

	// A cleared field binds to null and a decimal reaches the server's strict decoder (R4-007): check here, in words.
	const whole = (n: unknown) => typeof n === 'number' && Number.isSafeInteger(n) && n >= 0;
	async function save(e: SubmitEvent) {
		e.preventDefault();
		if (!address) return;
		error = '';
		if (!whole(editBan) || !whole(editChallenge)) {
			error = 'Both counters are whole numbers from 0.';
			return;
		}
		busy = true;
		try {
			await withReauth(() =>
				api.PATCH('/api/addresses/{address}/counters', {
					params: { path: { address: address!.split('/')[0] ?? address! } },
					body: { ban_count: editBan, challenge_count: editChallenge }
				})
			);
			address = null;
			await onsaved?.();
		} catch (e) {
			error = problemText(e);
		} finally {
			busy = false;
		}
	}
</script>

<Dialog.Root open={address !== null} onOpenChange={(o) => !o && (address = null)}>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title>Counters for <span class="font-mono">{shown}</span></Dialog.Title>
			<Dialog.Description
				>A running ban keeps its end time; the new ban counter applies to the next ban.</Dialog.Description
			>
		</Dialog.Header>
		<form class="grid gap-4" onsubmit={save}>
			<div class="grid grid-cols-2 gap-3">
				<div class="space-y-1.5">
					<Label for="eb">Ban counter</Label><Input
						id="eb"
						type="number"
						min="0"
						step="1"
						bind:value={editBan}
					/>
				</div>
				<div class="space-y-1.5">
					<Label for="ec">Challenge counter</Label><Input
						id="ec"
						type="number"
						min="0"
						step="1"
						bind:value={editChallenge}
					/>
				</div>
			</div>
			{#if error}<p role="alert" class="text-md text-bad-text">{error}</p>{/if}
			<Dialog.Footer>
				<Button type="button" variant="ghost" onclick={() => (address = null)}>Cancel</Button>
				<Button type="submit" disabled={busy}><KeyRound />Confirm with passkey and save</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
