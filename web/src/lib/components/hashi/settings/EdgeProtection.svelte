<script lang="ts">
	// Settings → Edge protection (10c §1.2): what applies to every service at the edge. The Security page shows what
	// happened; sign-in providers and the GeoIP source are on Connections.
	import ChallengeSettings from '#lib/components/hashi/security/ChallengeSettings.svelte';
	import EngineSummary from '#lib/components/hashi/security/EngineSummary.svelte';
	import EdgeSessions from './EdgeSessions.svelte';
</script>

<div class="space-y-4">
	<EngineSummary />
	<div class="grid items-start gap-4 lg:grid-cols-2">
		<ChallengeSettings />
		<EdgeSessions />
	</div>
</div>
