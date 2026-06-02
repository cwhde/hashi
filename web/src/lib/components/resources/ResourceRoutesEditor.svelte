<script lang="ts">
	import type { ResourceRouteRequest } from '$lib/components/resources/resource-routes';
	import { createEmptyRoute, removeRoute, reorderRoute } from '$lib/components/resources/resource-routes';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Switch } from '$lib/components/ui/switch';
	import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-svelte';

	let {
		title = 'Advanced routes',
		routes = $bindable<ResourceRouteRequest[]>([]),
		availableMiddlewares = [],
		baseTarget = { targetScheme: 'https', targetHost: '', targetPort: 443 },
		disabled = false
	}: {
		title?: string;
		routes?: ResourceRouteRequest[];
		availableMiddlewares?: string[];
		baseTarget?: Pick<ResourceRouteRequest, 'targetScheme' | 'targetHost' | 'targetPort'>;
		disabled?: boolean;
	} = $props();

	function addRoute() {
		routes = [...routes, createEmptyRoute(baseTarget)];
	}

	function moveRoute(index: number, direction: -1 | 1) {
		routes = reorderRoute(routes, index, direction);
	}

	function deleteRoute(index: number) {
		routes = removeRoute(routes, index);
	}

	function setMiddleware(routeIndex: number, middleware: string, enabled: boolean) {
		const current = routes[routeIndex];
		if (!current) return;
		const existing = current.extraMiddlewares ?? [];
		const next = enabled
			? [...new Set([...existing, middleware])]
			: existing.filter((name) => name !== middleware);
		routes = routes.map((route, idx) =>
			idx === routeIndex ? { ...route, extraMiddlewares: next } : route
		);
	}

	function setRewriteMode(routeIndex: number, value: string) {
		routes = routes.map((route, idx) =>
			idx === routeIndex
				? {
						...route,
						rewriteMode: value || null,
						rewriteValue: value ? route.rewriteValue : null
					}
				: route
		);
	}
</script>

<div class="grid gap-3 rounded-md border border-border p-3">
	<div class="flex items-center justify-between">
		<div>
			<p class="text-sm font-medium text-white">{title}</p>
			<p class="text-xs text-muted-foreground">
				Route order is highest priority first. Each route can override target and middleware chain.
			</p>
		</div>
		<Button size="sm" variant="outline" disabled={disabled} onclick={addRoute}>
			<Plus class="mr-1 size-4" />
			Add route
		</Button>
	</div>

	{#if routes.length === 0}
		<p class="text-xs text-muted-foreground">
			No advanced routes configured. Traffic falls back to the resource target.
		</p>
	{:else}
		<div class="grid gap-3">
			{#each routes as route, routeIndex (routeIndex)}
				<div class="grid gap-3 rounded-md border border-border/70 bg-muted/20 p-3">
					<div class="flex flex-wrap items-center justify-between gap-2">
						<div class="flex items-center gap-2">
							<Switch bind:checked={route.enabled} disabled={disabled} />
							<span class="text-xs font-medium text-white">Route {routeIndex + 1}</span>
						</div>
						<div class="flex items-center gap-1">
							<Button
								variant="ghost"
								size="icon-sm"
								disabled={disabled || routeIndex === 0}
								onclick={() => moveRoute(routeIndex, -1)}
							>
								<ArrowUp class="size-4" />
							</Button>
							<Button
								variant="ghost"
								size="icon-sm"
								disabled={disabled || routeIndex === routes.length - 1}
								onclick={() => moveRoute(routeIndex, 1)}
							>
								<ArrowDown class="size-4" />
							</Button>
							<Button
								variant="ghost"
								size="icon-sm"
								disabled={disabled}
								onclick={() => deleteRoute(routeIndex)}
							>
								<Trash2 class="size-4 text-destructive" />
							</Button>
						</div>
					</div>

					<div class="grid grid-cols-4 gap-3">
						<div class="grid gap-1.5">
							<Label for={`route-priority-${routeIndex}`}>Priority</Label>
							<Input
								id={`route-priority-${routeIndex}`}
								type="number"
								inputmode="numeric"
								bind:value={route.priority}
								disabled={disabled}
							/>
						</div>
						<div class="col-span-3 grid gap-1.5">
							<Label for={`route-path-${routeIndex}`}>Path</Label>
							<Input
								id={`route-path-${routeIndex}`}
								bind:value={route.pathValue}
								placeholder="/api"
								disabled={disabled}
							/>
						</div>
					</div>

					<div class="grid grid-cols-2 gap-3">
						<div class="grid gap-1.5">
							<Label for={`route-match-${routeIndex}`}>Match type</Label>
							<select
								id={`route-match-${routeIndex}`}
								class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
								bind:value={route.pathMatchType}
								disabled={disabled}
							>
								<option value="prefix">Prefix</option>
								<option value="exact">Exact</option>
								<option value="regex">Regex</option>
							</select>
						</div>
						<div class="grid gap-1.5">
							<Label for={`route-rewrite-mode-${routeIndex}`}>Rewrite mode</Label>
							<select
								id={`route-rewrite-mode-${routeIndex}`}
								class="h-9 rounded-md border border-border bg-background px-3 text-sm text-white"
								value={route.rewriteMode ?? ''}
								onchange={(e) => setRewriteMode(routeIndex, (e.currentTarget as HTMLSelectElement).value)}
								disabled={disabled}
							>
								<option value="">None</option>
								<option value="replace_path">Replace path</option>
								<option value="replace_prefix">Replace prefix</option>
								<option value="strip_prefix">Strip prefix</option>
								<option value="regex">Regex replace</option>
							</select>
						</div>
					</div>

					{#if route.rewriteMode}
						<div class="grid gap-1.5">
							<Label for={`route-rewrite-value-${routeIndex}`}>Rewrite value</Label>
							<Input
								id={`route-rewrite-value-${routeIndex}`}
								bind:value={route.rewriteValue}
								placeholder={route.rewriteMode === 'regex' ? '^/api/(.*) => /v1/$1' : '/'}
								disabled={disabled}
							/>
						</div>
					{/if}

					<div class="grid grid-cols-3 gap-3">
						<div class="grid gap-1.5">
							<Label for={`route-scheme-${routeIndex}`}>Target scheme</Label>
							<Input id={`route-scheme-${routeIndex}`} bind:value={route.targetScheme} disabled={disabled} />
						</div>
						<div class="grid gap-1.5">
							<Label for={`route-host-${routeIndex}`}>Target host</Label>
							<Input id={`route-host-${routeIndex}`} bind:value={route.targetHost} disabled={disabled} />
						</div>
						<div class="grid gap-1.5">
							<Label for={`route-port-${routeIndex}`}>Target port</Label>
							<Input
								id={`route-port-${routeIndex}`}
								type="number"
								inputmode="numeric"
								bind:value={route.targetPort}
								disabled={disabled}
							/>
						</div>
					</div>

					{#if availableMiddlewares.length > 0}
						<div class="grid gap-1.5">
							<span class="text-xs font-medium text-muted-foreground">Route middlewares</span>
							<div class="flex flex-wrap gap-x-4 gap-y-2">
								{#each availableMiddlewares as middleware (middleware)}
									<label class="flex items-center gap-2 text-xs text-muted-foreground">
										<input
											type="checkbox"
											disabled={disabled}
											checked={(route.extraMiddlewares ?? []).includes(middleware)}
											onchange={(e) =>
												setMiddleware(
													routeIndex,
													middleware,
													(e.currentTarget as HTMLInputElement).checked
												)}
										/>
										{middleware}
									</label>
								{/each}
							</div>
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>
