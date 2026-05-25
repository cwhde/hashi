<script lang="ts">
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import { Cable } from 'lucide-svelte';
</script>

<AdminSectionPage
	title="Connections"
	description="DNS providers, Traefik SSH, AdGuard, notifications, and SSO."
	icon={Cable}
	pendingFeature="Waiting for Connections API"
	pendingDetail="Connection registry endpoints will wire here when backend publishes /api/connections/*."
/>
