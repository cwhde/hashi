<script lang="ts">
	import { onMount } from 'svelte';
	import { navigate } from '$lib/navigation';
	import { api, ApiRequestError } from '$lib/api/client';
	import type { SetupStatusResponse } from '$lib/api/types';
	import { SETUP_STEPS, stepBySlug } from '$lib/setup/steps';
	import SetupStepNav from '$lib/components/setup/SetupStepNav.svelte';
	import SetupStepShell from '$lib/components/setup/SetupStepShell.svelte';
	import BootstrapAccessStep from '$lib/components/setup/steps/BootstrapAccessStep.svelte';
	import BaseSettingsStep from '$lib/components/setup/steps/BaseSettingsStep.svelte';
	import DnsProviderStep from '$lib/components/setup/steps/DnsProviderStep.svelte';
	import CertificateProviderStep from '$lib/components/setup/steps/CertificateProviderStep.svelte';
	import TraefikConnectionStep from '$lib/components/setup/steps/TraefikConnectionStep.svelte';
	import FirewallHostStep from '$lib/components/setup/steps/FirewallHostStep.svelte';
	import SystemResourceStep from '$lib/components/setup/steps/SystemResourceStep.svelte';
	import PasskeyVaultStep from '$lib/components/setup/steps/PasskeyVaultStep.svelte';
	import OptionalStep from '$lib/components/setup/steps/OptionalStep.svelte';
	import CompleteStep from '$lib/components/setup/steps/CompleteStep.svelte';
	import { Alert, AlertDescription, AlertTitle } from '$lib/components/ui/alert';
	import { LoaderCircle } from 'lucide-svelte';

	let status = $state<SetupStatusResponse | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let advancing = $state(false);

	const currentSlug = $derived(status?.currentStep ?? 'bootstrap-access');
	const currentStep = $derived(stepBySlug(currentSlug) ?? SETUP_STEPS[0]);
	const completedSteps = $derived(status?.completedSteps ?? []);

	onMount(() => {
		void loadStatus();
	});

	async function loadStatus() {
		loading = true;
		error = null;
		try {
			status = await api.getSetupStatus();
			if (status.isComplete) {
				await navigate('/');
			}
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to load setup status';
		} finally {
			loading = false;
		}
	}

	async function completeStep(slug: string) {
		advancing = true;
		error = null;
		try {
			status = await api.completeSetupStep(slug);
			if (status.isComplete || slug === 'complete') {
				await navigate('/');
			}
		} catch (e) {
			error = e instanceof ApiRequestError ? e.message : 'Failed to advance setup';
		} finally {
			advancing = false;
		}
	}
</script>

<div class="mx-auto flex min-h-[calc(100vh-3rem)] max-w-6xl gap-6 p-6">
	<aside class="hidden w-56 shrink-0 lg:block">
		<p class="mb-3 text-xs font-medium uppercase tracking-wide text-hashi-contrast">Setup wizard</p>
		<SetupStepNav currentSlug={currentSlug} {completedSteps} />
	</aside>

	<main class="min-w-0 flex-1">
		{#if loading}
			<div class="flex items-center gap-2 text-sm text-muted-foreground">
				<LoaderCircle class="size-4 animate-spin" />
				Loading setup state…
			</div>
		{:else if error}
			<Alert variant="destructive">
				<AlertTitle>Setup error</AlertTitle>
				<AlertDescription>{error}</AlertDescription>
			</Alert>
		{:else}
			<SetupStepShell step={currentStep}>
				{#if currentSlug === 'bootstrap-access'}
					<BootstrapAccessStep oncomplete={() => completeStep('bootstrap-access')} {advancing} />
				{:else if currentSlug === 'base-settings'}
					<BaseSettingsStep oncomplete={() => completeStep('base-settings')} {advancing} />
				{:else if currentSlug === 'dns-provider'}
					<DnsProviderStep oncomplete={() => completeStep('dns-provider')} {advancing} />
				{:else if currentSlug === 'certificate-provider'}
					<CertificateProviderStep
						oncomplete={() => completeStep('certificate-provider')}
						{advancing}
					/>
				{:else if currentSlug === 'traefik-connection'}
					<TraefikConnectionStep
						oncomplete={() => completeStep('traefik-connection')}
						{advancing}
					/>
				{:else if currentSlug === 'firewall-host'}
					<FirewallHostStep oncomplete={() => completeStep('firewall-host')} {advancing} />
				{:else if currentSlug === 'system-resource'}
					<SystemResourceStep oncomplete={() => completeStep('system-resource')} {advancing} />
				{:else if currentSlug === 'passkey-and-vault'}
					<PasskeyVaultStep oncomplete={() => completeStep('passkey-and-vault')} {advancing} />
				{:else if currentSlug === 'optional'}
					<OptionalStep oncomplete={() => completeStep('optional')} {advancing} />
				{:else}
					<CompleteStep oncomplete={() => completeStep('complete')} {advancing} />
				{/if}
			</SetupStepShell>
		{/if}
	</main>
</div>
