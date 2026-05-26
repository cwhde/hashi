<script lang="ts">
	import { navigate } from '$lib/navigation';
	import { onMount } from 'svelte';
	import { api, ApiRequestError } from '$lib/api/client';
	import {
		extractPrfOutput,
		isWebAuthnSupported,
		loginPasskeyFromServerOptions,
		serializeAuthentication
	} from '$lib/auth/webauthn';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Alert, AlertDescription, AlertTitle } from '$lib/components/ui/alert';
	import { KeyRound, Lock } from 'lucide-svelte';

	let mode = $state<'passkey' | 'bootstrap'>('passkey');
	let username = $state('');
	let password = $state('');
	let error = $state<string | null>(null);
	let loading = $state(false);
	let webauthnOk = $state(false);

	onMount(async () => {
		webauthnOk = isWebAuthnSupported();
		try {
			const session = await api.getSession();
			if (session.isAuthenticated) {
				await navigate(session.setupComplete ? '/' : '/setup');
			}
		} catch {
			// offline dev
		}
	});

	async function passkeyLogin() {
		loading = true;
		error = null;
		try {
			const begin = await api.passkeyLoginBegin();
			const options = begin.options as Record<string, unknown>;
			const challengeSessionId = String(begin.challengeSessionId ?? '');
			const credential = await loginPasskeyFromServerOptions(options);
			const assertion = serializeAuthentication(credential);
			const prfOutput = extractPrfOutput(credential);
			const result = await api.passkeyLoginComplete(assertion, challengeSessionId, prfOutput);
			if (!result.succeeded) {
				error = 'Passkey verification failed.';
				return;
			}
			await navigate('/');
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Passkey login failed';
		} finally {
			loading = false;
		}
	}

	async function bootstrapLogin() {
		loading = true;
		error = null;
		try {
			const result = await api.bootstrapLogin(username, password);
			if (!result.succeeded) {
				error = result.error ?? 'Invalid credentials';
				return;
			}
			await navigate('/setup');
		} catch (e) {
			error =
				e instanceof ApiRequestError && e.status === 401
					? 'Invalid credentials'
					: e instanceof Error
						? e.message
						: 'Login failed';
		} finally {
			loading = false;
		}
	}
</script>

<div class="flex min-h-screen items-center justify-center p-6">
	<div class="w-full max-w-md space-y-6 rounded-lg border border-border bg-card/60 p-6">
		<div class="space-y-1 text-center">
			<h1 class="text-xl font-semibold text-white">Hashi Admin</h1>
			<p class="text-sm text-muted-foreground">Passkey-authenticated edge orchestration</p>
		</div>

		<div class="flex gap-2">
			<Button
				variant={mode === 'passkey' ? 'default' : 'outline'}
				class="flex-1"
				onclick={() => (mode = 'passkey')}
			>
				Passkey
			</Button>
			<Button
				variant={mode === 'bootstrap' ? 'default' : 'outline'}
				class="flex-1"
				onclick={() => (mode = 'bootstrap')}
			>
				Bootstrap
			</Button>
		</div>

		{#if mode === 'passkey'}
			<Alert>
				<AlertTitle>Passkey login</AlertTitle>
				<AlertDescription>
					Use your registered passkey. PRF unlocks the vault automatically when supported.
				</AlertDescription>
			</Alert>
			<Button class="w-full" disabled={!webauthnOk || loading} onclick={() => passkeyLogin()}>
				<KeyRound class="size-4" />
				{loading ? 'Authenticating…' : 'Sign in with passkey'}
			</Button>
		{:else}
			<form
				class="space-y-3"
				onsubmit={(e) => {
					e.preventDefault();
					void bootstrapLogin();
				}}
			>
				<div class="grid gap-1.5">
					<Label for="login-user">Username</Label>
					<Input id="login-user" bind:value={username} autocomplete="username" />
				</div>
				<div class="grid gap-1.5">
					<Label for="login-pass">Password</Label>
					<Input
						id="login-pass"
						type="password"
						bind:value={password}
						autocomplete="current-password"
					/>
				</div>
				<Button type="submit" class="w-full" disabled={loading}>
					<Lock class="size-4" />
					{loading ? 'Signing in…' : 'Bootstrap sign in'}
				</Button>
				<p class="text-[11px] text-muted-foreground">
					Bootstrap login is only available during setup from private networks.
				</p>
			</form>
		{/if}

		{#if error}
			<p class="text-center text-xs text-destructive">{error}</p>
		{/if}
	</div>
</div>
