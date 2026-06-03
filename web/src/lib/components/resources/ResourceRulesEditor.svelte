<script lang="ts">
	import type { ResourceRuleRequest } from '$lib/components/resources/resource-rules';
	import {
		RESOURCE_RULE_ACTIONS,
		RESOURCE_RULE_MATCH_TYPES,
		createEmptyRule,
		removeRule,
		reorderRule
	} from '$lib/components/resources/resource-rules';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-svelte';

	let {
		title = 'Resource rules',
		rules = $bindable<ResourceRuleRequest[]>([]),
		disabled = false
	}: {
		title?: string;
		rules?: ResourceRuleRequest[];
		disabled?: boolean;
	} = $props();

	function addRule() {
		rules = [...rules, createEmptyRule()];
	}

	function moveRule(index: number, direction: -1 | 1) {
		rules = reorderRule(rules, index, direction);
	}

	function deleteRule(index: number) {
		rules = removeRule(rules, index);
	}
</script>

<div class="grid gap-3 rounded-md border border-border p-3">
	<div class="flex items-center justify-between gap-3">
		<div>
			<p class="text-sm font-medium text-white">{title}</p>
		</div>
		<Button size="sm" variant="outline" disabled={disabled} onclick={addRule}>
			<Plus class="mr-1 size-4" />
			Add rule
		</Button>
	</div>

	{#if rules.length === 0}
		<p class="text-xs text-muted-foreground">No resource rules configured.</p>
	{:else}
		<div class="grid gap-3">
			{#each rules as rule, ruleIndex (ruleIndex)}
				<div class="grid gap-3 rounded-md border border-border/70 bg-muted/20 p-3">
					<div class="flex flex-wrap items-center justify-between gap-2">
						<div class="flex items-center gap-2">
							<Switch bind:checked={rule.enabled} disabled={disabled} />
							<span class="text-xs font-medium text-white">Rule {ruleIndex + 1}</span>
						</div>
						<div class="flex items-center gap-1">
							<Button
								variant="ghost"
								size="icon-sm"
								disabled={disabled || ruleIndex === 0}
								onclick={() => moveRule(ruleIndex, -1)}
							>
								<ArrowUp class="size-4" />
							</Button>
							<Button
								variant="ghost"
								size="icon-sm"
								disabled={disabled || ruleIndex === rules.length - 1}
								onclick={() => moveRule(ruleIndex, 1)}
							>
								<ArrowDown class="size-4" />
							</Button>
							<Button
								variant="ghost"
								size="icon-sm"
								disabled={disabled}
								onclick={() => deleteRule(ruleIndex)}
							>
								<Trash2 class="size-4 text-destructive" />
							</Button>
						</div>
					</div>

					<div class="grid grid-cols-4 gap-3">
						<div class="grid gap-1.5">
							<Label for={`rule-priority-${ruleIndex}`}>Priority</Label>
							<Input
								id={`rule-priority-${ruleIndex}`}
								type="number"
								inputmode="numeric"
								bind:value={rule.priority}
								disabled={disabled}
							/>
						</div>
						<div class="col-span-3 grid gap-1.5">
							<Label for={`rule-action-${ruleIndex}`}>Action</Label>
							<select
								id={`rule-action-${ruleIndex}`}
								class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
								bind:value={rule.action}
								disabled={disabled}
							>
								{#each RESOURCE_RULE_ACTIONS as action (action.value)}
									<option value={action.value}>{action.label}</option>
								{/each}
							</select>
						</div>
					</div>

					<div class="grid grid-cols-3 gap-3">
						<div class="grid gap-1.5">
							<Label for={`rule-match-type-${ruleIndex}`}>Match type</Label>
							<select
								id={`rule-match-type-${ruleIndex}`}
								class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
								bind:value={rule.matchType}
								disabled={disabled}
							>
								{#each RESOURCE_RULE_MATCH_TYPES as matchType (matchType.value)}
									<option value={matchType.value}>{matchType.label}</option>
								{/each}
							</select>
						</div>
						<div class="col-span-2 grid gap-1.5">
							<Label for={`rule-match-value-${ruleIndex}`}>Match value</Label>
							<Input
								id={`rule-match-value-${ruleIndex}`}
								bind:value={rule.matchValue}
								placeholder={rule.matchType === 'path' ? '/admin' : '203.0.113.0/24'}
								disabled={disabled}
							/>
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
