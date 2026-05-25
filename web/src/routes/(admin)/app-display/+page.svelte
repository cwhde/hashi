<script lang="ts">
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import { Monitor } from 'lucide-svelte';
</script>

<AdminSectionPage
	title="App Display"
	description="Public dashboard tiles, grouping, and port 8081 presentation."
	icon={Monitor}
	pendingFeature="Waiting for App Display API"
	pendingDetail="Public dashboard configuration endpoints will wire here when backend publishes /api/app-display/*."
/>
