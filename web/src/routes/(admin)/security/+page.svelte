<script lang="ts">
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import { Lock } from 'lucide-svelte';
</script>

<AdminSectionPage
	title="Security"
	description="Adaptive abuse response, blocks, SSO profiles, and security events."
	icon={Lock}
	pendingFeature="Waiting for Security events API"
	pendingDetail="Security dashboard endpoints will wire here when backend publishes /api/security/*."
/>
