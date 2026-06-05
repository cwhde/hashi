<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';

	let saving = $state(false);
	let testing = $state(false);
	let message = $state<string | null>(null);
	let testToken = $state('');
	let form = $state({
		enabled: false,
		publicChallengeBaseUrl: '',
		siteKey: '',
		secretKey: '',
		hasSecretKey: false,
		verificationTimeoutSeconds: 5,
		instrumentationExpected: true,
		headlessDetectionExpected: false,
		publicChallengeDomain: '',
		publicChallengeResourceId: null as string | null,
		capAdminDomain: '',
		capAdminResourceId: null as string | null,
		challengeResetMode: 'decay',
		challengeDecayPercent: 50,
		minimumRepeatChallengeSeconds: 300,
		maximumFailuresBeforeEscalation: 5,
		maximumRequestsWhileChallenged: 30
	});

	onMount(load);

	async function load() {
		const settings = await api.getCaptchaSettings();
		form = {
			enabled: settings.enabled,
			publicChallengeBaseUrl: settings.publicChallengeBaseUrl ?? '',
			siteKey: settings.siteKey ?? '',
			secretKey: '',
			hasSecretKey: settings.hasSecretKey,
			verificationTimeoutSeconds: settings.verificationTimeoutSeconds,
			instrumentationExpected: settings.instrumentationExpected,
			headlessDetectionExpected: settings.headlessDetectionExpected,
			publicChallengeDomain: settings.publicChallengeDomain ?? '',
			publicChallengeResourceId: settings.publicChallengeResourceId,
			capAdminDomain: settings.capAdminDomain ?? '',
			capAdminResourceId: settings.capAdminResourceId,
			challengeResetMode: settings.challengeResetMode,
			challengeDecayPercent: settings.challengeDecayPercent,
			minimumRepeatChallengeSeconds: settings.minimumRepeatChallengeSeconds,
			maximumFailuresBeforeEscalation: settings.maximumFailuresBeforeEscalation,
			maximumRequestsWhileChallenged: settings.maximumRequestsWhileChallenged
		};
	}

	async function save() {
		saving = true;
		message = null;
		try {
			const saved = await api.updateCaptchaSettings({
				enabled: form.enabled,
				publicChallengeBaseUrl: form.publicChallengeBaseUrl || null,
				siteKey: form.siteKey || null,
				secretKey: form.secretKey || null,
				secretKeySecretId: null,
				verificationTimeoutSeconds: form.verificationTimeoutSeconds,
				instrumentationExpected: form.instrumentationExpected,
				headlessDetectionExpected: form.headlessDetectionExpected,
				publicChallengeResourceId: form.publicChallengeResourceId,
				publicChallengeDomain: form.publicChallengeDomain || null,
				capAdminResourceId: form.capAdminResourceId,
				capAdminDomain: form.capAdminDomain || null,
				challengeResetMode: form.challengeResetMode,
				challengeDecayPercent: form.challengeDecayPercent,
				minimumRepeatChallengeSeconds: form.minimumRepeatChallengeSeconds,
				maximumFailuresBeforeEscalation: form.maximumFailuresBeforeEscalation,
				maximumRequestsWhileChallenged: form.maximumRequestsWhileChallenged
			});
			form.secretKey = '';
			form.hasSecretKey = saved.hasSecretKey;
			form.publicChallengeResourceId = saved.publicChallengeResourceId;
			form.capAdminResourceId = saved.capAdminResourceId;
			message = 'CAPTCHA settings saved.';
		} catch (e) {
			message = e instanceof Error ? e.message : 'Failed to save CAPTCHA settings';
		} finally {
			saving = false;
		}
	}

	async function testTokenOnce() {
		testing = true;
		message = null;
		try {
			const result = await api.testCaptchaToken(testToken);
			message = result.succeeded ? 'Cap token verified.' : (result.error ?? 'Cap token failed.');
		} catch (e) {
			message = e instanceof Error ? e.message : 'Cap test failed';
		} finally {
			testing = false;
		}
	}
</script>

<div class="grid gap-4">
	<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
		<div>
			<p class="text-sm text-white">Cap integration</p>
			<p class="text-xs text-muted-foreground">
				{form.publicChallengeResourceId
					? `Required public resource: ${form.publicChallengeResourceId}`
					: 'Required public challenge resource will be created when enabled'}
			</p>
		</div>
		<Switch bind:checked={form.enabled} />
	</div>

	<div class="grid grid-cols-2 gap-3">
		<div class="grid gap-1.5">
			<Label for="captcha-base">Cap public base URL</Label>
			<Input id="captcha-base" bind:value={form.publicChallengeBaseUrl} placeholder="https://cap.example.com" />
		</div>
		<div class="grid gap-1.5">
			<Label for="captcha-site-key">Site key</Label>
			<Input id="captcha-site-key" bind:value={form.siteKey} />
		</div>
	</div>

	<div class="grid grid-cols-2 gap-3">
		<div class="grid gap-1.5">
			<Label for="captcha-secret">Secret key{form.hasSecretKey ? ' stored' : ''}</Label>
			<Input id="captcha-secret" type="password" bind:value={form.secretKey} />
		</div>
		<div class="grid gap-1.5">
			<Label for="captcha-timeout">Verify timeout (seconds)</Label>
			<Input id="captcha-timeout" type="number" min="1" max="30" bind:value={form.verificationTimeoutSeconds} />
		</div>
	</div>

	<div class="grid grid-cols-2 gap-3">
		<div class="grid gap-1.5">
			<Label for="captcha-public-domain">Public challenge domain</Label>
			<Input id="captcha-public-domain" bind:value={form.publicChallengeDomain} placeholder="challenge.example.com" />
		</div>
		<div class="grid gap-1.5">
			<Label for="captcha-admin-domain">Optional Cap admin domain</Label>
			<Input id="captcha-admin-domain" bind:value={form.capAdminDomain} placeholder="cap-admin.example.com" />
		</div>
	</div>

	<div class="grid grid-cols-2 gap-3">
		<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
			<span class="text-sm text-white">Instrumentation expected</span>
			<Switch bind:checked={form.instrumentationExpected} />
		</div>
		<div class="flex items-center justify-between rounded-md border border-border px-3 py-2">
			<span class="text-sm text-white">Headless detection expected</span>
			<Switch bind:checked={form.headlessDetectionExpected} />
		</div>
	</div>

	<div class="grid gap-3 sm:grid-cols-2">
		<div class="grid gap-1.5">
			<Label for="captcha-reset-mode">Solve behavior</Label>
			<select
				id="captcha-reset-mode"
				class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
				bind:value={form.challengeResetMode}
			>
				<option value="decay">Decay buckets</option>
				<option value="reset">Reset buckets</option>
				<option value="none">Keep buckets</option>
			</select>
		</div>
		<div class="grid gap-1.5">
			<Label for="captcha-decay">Decay percent</Label>
			<Input id="captcha-decay" type="number" min="0" max="100" bind:value={form.challengeDecayPercent} />
		</div>
		<div class="grid gap-1.5">
			<Label for="captcha-failures">Failure threshold</Label>
			<Input id="captcha-failures" type="number" min="1" bind:value={form.maximumFailuresBeforeEscalation} />
		</div>
		<div class="grid gap-1.5">
			<Label for="captcha-ignored">Ignored threshold</Label>
			<Input id="captcha-ignored" type="number" min="1" bind:value={form.maximumRequestsWhileChallenged} />
		</div>
	</div>

	<div class="grid grid-cols-[1fr_auto] gap-2">
		<Input bind:value={testToken} placeholder="One-use Cap token for server-side verification test" />
		<Button variant="outline" onclick={() => testTokenOnce()} disabled={testing || !testToken}>
			{testing ? 'Testing...' : 'Test token'}
		</Button>
	</div>

	{#if message}
		<p class="text-xs text-muted-foreground">{message}</p>
	{/if}

	<Button onclick={() => save()} disabled={saving}>{saving ? 'Saving...' : 'Save CAPTCHA settings'}</Button>
</div>
