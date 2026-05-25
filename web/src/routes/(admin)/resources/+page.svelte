<script lang="ts">
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import { Server } from 'lucide-svelte';
</script>

<AdminSectionPage
	title="Resources"
	description="Managed homelab services, routing targets, and health probes."
	icon={Server}
	pendingFeature="Waiting for Resources API"
	pendingDetail="Resource CRUD and sync preview endpoints will wire here when backend publishes /api/resources/*."
/>
