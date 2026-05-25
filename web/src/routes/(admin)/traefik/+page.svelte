<script lang="ts">
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import { Network } from 'lucide-svelte';
</script>

<AdminSectionPage
	title="Traefik"
	description="Connection health, config ownership, routers, and reconcile state."
	icon={Network}
	pendingFeature="Waiting for Traefik sync API"
	pendingDetail="Traefik connection and reconcile endpoints will wire here when backend publishes /api/traefik/*."
/>
