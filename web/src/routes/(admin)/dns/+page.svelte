<script lang="ts">
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import { Globe } from 'lucide-svelte';
</script>

<AdminSectionPage
	title="DNS"
	description="Provider zones, managed records, sync plans, and import controls."
	icon={Globe}
	pendingFeature="Waiting for DNS sync API"
	pendingDetail="List, plan, and apply endpoints will wire here when backend publishes /api/dns/*."
/>
