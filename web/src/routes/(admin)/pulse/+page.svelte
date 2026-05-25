<script lang="ts">
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import { Zap } from 'lucide-svelte';
</script>

<AdminSectionPage
	title="Pulse"
	description="Dynamic endpoint agents, discovery tokens, and last-seen endpoints."
	icon={Zap}
	pendingFeature="Waiting for Pulse agent API"
	pendingDetail="Pulse agent endpoints will wire here when backend publishes /api/pulse/*."
/>
