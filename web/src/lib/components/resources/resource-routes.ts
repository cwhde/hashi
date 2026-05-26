import type { Schemas } from '$lib/api/types';

export type ResourceRouteRequest = Schemas['ResourceRouteRequest'];

export const DEFAULT_ROUTE_PRIORITY = 100;

export function createEmptyRoute(
	baseTarget: Pick<ResourceRouteRequest, 'targetScheme' | 'targetHost' | 'targetPort'>
): ResourceRouteRequest {
	return {
		enabled: true,
		priority: DEFAULT_ROUTE_PRIORITY,
		pathMatchType: 'prefix',
		pathValue: '/',
		targetScheme: baseTarget.targetScheme || 'https',
		targetHost: baseTarget.targetHost || '',
		targetPort: Number(baseTarget.targetPort) || 443,
		rewriteMode: null,
		rewriteValue: null,
		extraMiddlewares: []
	};
}

export function reorderRoute(
	routes: ResourceRouteRequest[],
	index: number,
	direction: -1 | 1
): ResourceRouteRequest[] {
	const nextIndex = index + direction;
	if (index < 0 || nextIndex < 0 || index >= routes.length || nextIndex >= routes.length) {
		return routes;
	}

	const next = [...routes];
	const [item] = next.splice(index, 1);
	next.splice(nextIndex, 0, item);
	return next;
}

export function removeRoute(routes: ResourceRouteRequest[], index: number): ResourceRouteRequest[] {
	if (index < 0 || index >= routes.length) {
		return routes;
	}

	return routes.filter((_, routeIndex) => routeIndex !== index);
}

export function normalizeRoutes(routes: ResourceRouteRequest[]): ResourceRouteRequest[] {
	return routes.map((route) => {
		const rewriteMode = route.rewriteMode?.trim() || null;
		return {
			...route,
			priority: Number(route.priority),
			targetPort: Number(route.targetPort),
			pathMatchType: route.pathMatchType?.trim() || 'prefix',
			pathValue: route.pathValue?.trim() || '/',
			targetScheme: route.targetScheme?.trim() || 'https',
			targetHost: route.targetHost?.trim() || '',
			rewriteMode,
			rewriteValue: rewriteMode ? route.rewriteValue?.trim() || null : null,
			extraMiddlewares: route.extraMiddlewares ?? []
		};
	});
}
