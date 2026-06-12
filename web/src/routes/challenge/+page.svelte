<script lang="ts">
	import { onMount } from 'svelte';
	import { api, ApiRequestError } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';

	let status = $state<import('$lib/api/types').CaptchaChallengeStatus | null>(null);
	let returnUrl = $state<string | null>(null);
	let loading = $state(true);
	let verifying = $state(false);
	let message = $state<string | null>(null);
	const handleSolveEvent = (event: globalThis.Event) => {
		void handleSolve(event as CustomEvent<{ token?: string }>);
	};

	function listenForSolve(node: HTMLElement) {
		node.addEventListener('solve', handleSolveEvent);
		return {
			destroy() {
				node.removeEventListener('solve', handleSolveEvent);
			}
		};
	}

	onMount(() => {
		async function init() {
			returnUrl = new URL(window.location.href).searchParams.get('returnUrl');
			try {
				status = await api.getCaptchaChallengeStatus(returnUrl);
				await loadWidgetScript();
			} catch (e) {
				message = e instanceof Error ? e.message : 'Challenge unavailable';
			} finally {
				loading = false;
			}
		}

		void init();
	});

	async function loadWidgetScript() {
		if (document.querySelector('script[data-hashi-cap-widget]')) return;
		const script = document.createElement('script');
		script.type = 'module';
		script.src = 'https://cdn.jsdelivr.net/npm/cap-widget';
		script.dataset.hashiCapWidget = 'true';
		await new Promise<void>((resolve, reject) => {
			script.onload = () => resolve();
			script.onerror = () => reject(new Error('Failed to load CAPTCHA widget'));
			document.head.appendChild(script);
		});
	}

	async function handleSolve(event: CustomEvent<{ token?: string }>) {
		const token = event.detail?.token;
		if (!token) {
			message = 'Challenge token missing.';
			return;
		}

		verifying = true;
		message = null;
		try {
			const result = await api.verifyCaptchaChallenge(token, returnUrl);
			if (result.verified && result.redirectUrl) {
				window.location.assign(result.redirectUrl);
				return;
			}

			message = result.error ?? 'Challenge verification failed.';
		} catch (e) {
			message = e instanceof ApiRequestError ? e.message : 'Challenge verification failed.';
		} finally {
			verifying = false;
		}
	}
</script>

<main class="grid min-h-screen place-items-center bg-background px-4 py-8 text-foreground">
	<section class="grid w-full max-w-sm gap-5">
		<div class="grid gap-1">
			<h1 class="text-xl font-semibold text-white">Security Challenge</h1>
			<p class="break-words text-sm text-muted-foreground">
				{status?.safeReturnUrl && status.safeReturnUrl !== '/'
					? new URL(status.safeReturnUrl).host
					: 'Hashi protected resource'}
			</p>
		</div>

		{#if loading}
			<p class="text-sm text-muted-foreground">Loading challenge...</p>
		{:else if !status?.enabled || !status.capApiEndpoint}
			<p class="text-sm text-destructive">Challenge flow is not available.</p>
		{:else}
			<div class="grid min-h-16 place-items-start">
				<cap-widget use:listenForSolve data-cap-api-endpoint={status.capApiEndpoint}></cap-widget>
			</div>
			{#if verifying}
				<p class="text-sm text-muted-foreground">Verifying...</p>
			{/if}
		{/if}

		{#if message}
			<p class="text-sm text-destructive">{message}</p>
		{/if}

		{#if status?.safeReturnUrl && status.safeReturnUrl !== '/'}
			<Button variant="outline" href={status.safeReturnUrl}>Return</Button>
		{/if}
	</section>
</main>
