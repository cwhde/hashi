<script lang="ts">
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import { Shield } from 'lucide-svelte';
</script>

<AdminSectionPage
	title="Firewall Hosts"
	description="Linux edge hosts, managed subnets, port forwards, and Hashi chains."
	icon={Shield}
	pendingFeature="Waiting for Firewall Hosts API"
	pendingDetail="Firewall host management endpoints will wire here when backend publishes /api/firewall-hosts/*."
/>
