<script lang="ts">
	import AdminSectionPage from '$lib/components/layout/AdminSectionPage.svelte';
	import { FileCode } from 'lucide-svelte';
</script>

<AdminSectionPage
	title="Scripts"
	description="Privileged shell scripts, cron schedules, and manual run output."
	icon={FileCode}
	pendingFeature="Waiting for Scripts API"
	pendingDetail="Custom script endpoints will wire here when backend publishes /api/scripts/*."
/>
