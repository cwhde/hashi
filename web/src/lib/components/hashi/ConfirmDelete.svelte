<script lang="ts">
	// A delete that can't be undone asks first and names what is lost (10c §5, 02 §2 bug 51).
	import * as AlertDialog from '#lib/components/ui/alert-dialog/index.ts';

	let {
		open = $bindable(false),
		title,
		body,
		verb,
		onconfirm
	}: {
		open?: boolean;
		title: string;
		body: string;
		verb: string;
		onconfirm: () => void;
	} = $props();
</script>

<AlertDialog.Root bind:open>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{title}</AlertDialog.Title>
			<AlertDialog.Description>{body}</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>Keep it</AlertDialog.Cancel>
			<AlertDialog.Action
				class="border border-bad/30 bg-bad/15 text-bad-text hover:bg-bad/25"
				onclick={() => {
					open = false;
					onconfirm();
				}}>{verb}</AlertDialog.Action
			>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
