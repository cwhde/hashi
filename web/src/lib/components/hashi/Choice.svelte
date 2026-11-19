<script lang="ts" generics="T extends string">
	// The one dropdown for picking a value from a short list (bug 112): the app's Select, never the browser's own.
	// An option may have the value '' (for "none" or "the default"); Bits UI treats '' as "nothing chosen", so it is
	// carried under a stand-in value inside and handed back as ''.
	import * as Select from '#lib/components/ui/select/index.ts';
	import { cn } from '#lib/utils.ts';

	type Option = { value: T; label: string; disabled?: boolean };
	let {
		value = $bindable(),
		options,
		id,
		label,
		placeholder = 'Choose…',
		disabled = false,
		class: className = '',
		onchange
	}: {
		value?: T;
		options: Option[];
		id?: string;
		/** Accessible name when no <Label for={id}> names it. */
		label?: string;
		placeholder?: string;
		disabled?: boolean;
		class?: string;
		onchange?: (value: T) => void;
	} = $props();

	const NONE = '\u0000none';
	const inner = (v: string | undefined) => (!v ? NONE : v);
	const outer = (v: string) => (v === NONE ? '' : v) as T;
	const chosen = $derived(options.find((o) => o.value === value));
</script>

<Select.Root
	type="single"
	value={inner(value)}
	onValueChange={(v) => {
		const next = outer(v);
		value = next;
		onchange?.(next);
	}}
	{disabled}
>
	<Select.Trigger {id} aria-label={label} class={cn('w-full', className)}
		><span class={cn('truncate', !chosen && 'text-muted-foreground')}
			>{chosen?.label ?? placeholder}</span
		></Select.Trigger
	>
	<Select.Content>
		{#each options as o (o.value)}<Select.Item
				value={inner(o.value)}
				label={o.label}
				disabled={o.disabled}>{o.label}</Select.Item
			>{/each}
	</Select.Content>
</Select.Root>
