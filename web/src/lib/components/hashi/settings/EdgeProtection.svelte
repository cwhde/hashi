<script lang="ts">
	// Settings → Edge protection (10c §1.2): what applies to every service at the edge. The Security page shows what
	// happened; sign-in providers and the GeoIP source are on Connections.
	import ChallengeSettings from '#lib/components/hashi/security/ChallengeSettings.svelte';
	import EngineSummary from '#lib/components/hashi/security/EngineSummary.svelte';
	import EdgeSessions from './EdgeSessions.svelte';
</script>

<EngineSummary />
<ChallengeSettings />
<EdgeSessions />
