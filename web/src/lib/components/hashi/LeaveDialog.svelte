<script lang="ts">
	// The app's own "leave without saving?" question (bug 108), shown by the shell for every guarded form.
	import * as AlertDialog from '#lib/components/ui/alert-dialog/index.ts';
	import { leaveQuestion } from '#lib/unsaved.svelte.ts';
</script>

<AlertDialog.Root open={!!leaveQuestion.to} onOpenChange={(o) => !o && leaveQuestion.stay()}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>Leave without saving?</AlertDialog.Title>
			<AlertDialog.Description>Your changes on this page are not saved yet.</AlertDialog.Description
			>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>Stay</AlertDialog.Cancel>
			<AlertDialog.Action
				class="border border-bad/30 bg-bad/15 text-bad-text hover:bg-bad/25"
				onclick={() => leaveQuestion.leave()}>Leave</AlertDialog.Action
			>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
