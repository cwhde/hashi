<script lang="ts">
	import { onMount } from 'svelte';
	import { api, ApiRequestError } from '$lib/api/client';
	import ApiPendingBanner from '$lib/components/layout/ApiPendingBanner.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Alert, AlertDescription, AlertTitle } from '$lib/components/ui/alert';
	import { ShieldCheck } from 'lucide-svelte';

	let {
		oncomplete,
		advancing = false
	}: {
		oncomplete: () => void | Promise<void>;
		advancing?: boolean;
	} = $props();

	let username = $state('');
	let password = $state('');
	let allowed = $state<boolean | null>(null);
	let remoteIp = $state<string | null>(null);
	let loginError = $state<string | null>(null);
	let submitting = $state(false);

	onMount(async () => {
		try {
			const res = await api.getBootstrapAllowed();
			allowed = res.allowed;
			remoteIp = res.remoteIp;
		} catch {
			allowed = null;
		}
	});

	async function handleLogin() {
		loginError = null;
		submitting = true;
		try {
			if (!username || !password) {
				loginError = 'Enter bootstrap username and password from Docker logs.';
				return;
			}
			const result = await api.bootstrapLogin(username, password);
			if (!result.succeeded) {
				loginError = result.error ?? 'Invalid bootstrap credentials.';
				return;
			}
			await oncomplete();
		} catch (e) {
			loginError =
				e instanceof ApiRequestError
					? e.status === 401
						? 'Invalid bootstrap credentials.'
						: e.status === 403
							? 'Bootstrap login blocked from this network.'
							: e.message
					: 'Login failed';
		} finally {
			submitting = false;
		}
	}
</script>

<Alert>
	<AlertTitle>Bootstrap credentials</AlertTitle>
	<AlertDescription>
		On first boot Hashi prints a random username and password to Docker logs. Bootstrap login is
		restricted to private networks.
	</AlertDescription>
</Alert>

{#if allowed === false}
	<Alert variant="destructive" class="mt-3">
		<AlertTitle>Network blocked</AlertTitle>
		<AlertDescription>
			Your address ({remoteIp ?? 'unknown'}) is outside allowed private ranges. Connect via VPN or
			internal network.
		</AlertDescription>
	</Alert>
{:else if allowed === true}
	<div class="mt-3 flex items-center gap-2 text-xs text-emerald-300">
		<ShieldCheck class="size-4" />
		Private network access confirmed ({remoteIp ?? 'local'}).
	</div>
{/if}

<form
	class="mt-4 grid max-w-md gap-3"
	onsubmit={(e) => {
		e.preventDefault();
		void handleLogin();
	}}
>
	<div class="grid gap-1.5">
		<Label for="bootstrap-user">Username</Label>
		<Input id="bootstrap-user" bind:value={username} autocomplete="username" />
	</div>
	<div class="grid gap-1.5">
		<Label for="bootstrap-pass">Password</Label>
		<Input
			id="bootstrap-pass"
			type="password"
			bind:value={password}
			autocomplete="current-password"
		/>
	</div>
	{#if loginError}
		<p class="text-xs text-destructive">{loginError}</p>
	{/if}
	<div class="flex gap-2 pt-2">
		<Button type="submit" disabled={submitting || advancing}>
			{submitting ? 'Signing in…' : 'Sign in'}
		</Button>
	</div>
</form>

<ApiPendingBanner
	class="mt-4"
	message="Bootstrap session uses cookie auth"
	detail="Successful login creates an 8-hour setup session cookie for passkey registration."
/>
