<script lang="ts">
	// An upload control that is one control: a label drawn as a button around its file input, so the keyboard stops
	// once and a screen reader hears one named file picker. A file input inside a <button> was two nested controls.
	import { Upload } from '@lucide/svelte';
	import { buttonVariants } from '#lib/components/ui/button/index.ts';
	import { cn } from '#lib/utils.ts';

	let {
		accept,
		label,
		disabled = false,
		onchange
	}: { accept: string; label: string; disabled?: boolean; onchange: (e: Event) => void } = $props();
</script>

<label
	class={cn(
		buttonVariants({ variant: 'outline', size: 'sm' }),
		'relative cursor-pointer has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring',
		disabled && 'pointer-events-none opacity-50'
	)}
>
	<Upload />Upload
	<input
		type="file"
		{accept}
		{disabled}
		class="absolute inset-0 cursor-pointer opacity-0"
		aria-label={label}
		{onchange}
	/>
</label>
