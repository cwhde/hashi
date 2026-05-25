<script lang="ts">
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import { HeartPulse } from 'lucide-svelte';
</script>

<AdminSectionPage
	title="Status"
	description="Monitors, incidents, latency charts, and public status page config."
	icon={HeartPulse}
	pendingFeature="Waiting for Status monitors API"
	pendingDetail="Status monitor endpoints will wire here when backend publishes /api/status/*."
/>
