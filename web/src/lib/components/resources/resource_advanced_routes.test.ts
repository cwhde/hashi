import { describe, expect, it } from 'vitest';
import { createEmptyRoute, normalizeRoutes, removeRoute, reorderRoute } from './resource-routes';

describe('resource_advanced_routes', () => {
	it('creates a default route from base target', () => {
		const route = createEmptyRoute({
			targetScheme: 'https',
			targetHost: 'app.internal',
			targetPort: 8443
		});

		expect(route.pathMatchType).toBe('prefix');
		expect(route.pathValue).toBe('/');
		expect(route.targetHost).toBe('app.internal');
		expect(route.targetPort).toBe(8443);
		expect(route.rewriteMode).toBeNull();
	});

	it('reorders routes in requested direction', () => {
		const routes = [
			{ ...createEmptyRoute({ targetScheme: 'https', targetHost: 'a', targetPort: 443 }), pathValue: '/a' },
			{ ...createEmptyRoute({ targetScheme: 'https', targetHost: 'b', targetPort: 443 }), pathValue: '/b' }
		];

		const moved = reorderRoute(routes, 0, 1);
		expect(moved[0]?.pathValue).toBe('/b');
		expect(moved[1]?.pathValue).toBe('/a');
	});

	it('removes a route by index', () => {
		const routes = [
			{ ...createEmptyRoute({ targetScheme: 'https', targetHost: 'a', targetPort: 443 }), pathValue: '/a' },
			{ ...createEmptyRoute({ targetScheme: 'https', targetHost: 'b', targetPort: 443 }), pathValue: '/b' }
		];

		const reduced = removeRoute(routes, 0);
		expect(reduced).toHaveLength(1);
		expect(reduced[0]?.pathValue).toBe('/b');
	});

	it('normalizes numeric and optional rewrite fields', () => {
		const normalized = normalizeRoutes([
			{
				enabled: true,
				priority: '105',
				pathMatchType: ' exact ',
				pathValue: ' /admin ',
				targetScheme: ' https ',
				targetHost: ' app.internal ',
				targetPort: '9443',
				rewriteMode: '',
				rewriteValue: ' /ignored ',
				extraMiddlewares: null
			}
		]);

		expect(normalized[0]?.priority).toBe(105);
		expect(normalized[0]?.targetPort).toBe(9443);
		expect(normalized[0]?.pathMatchType).toBe('exact');
		expect(normalized[0]?.rewriteMode).toBeNull();
		expect(normalized[0]?.rewriteValue).toBeNull();
		expect(normalized[0]?.extraMiddlewares).toEqual([]);
	});

	it('preserves replace-prefix rewrite mode and target', () => {
		const normalized = normalizeRoutes([
			{
				...createEmptyRoute({ targetScheme: 'https', targetHost: 'app.internal', targetPort: 443 }),
				rewriteMode: ' replace_prefix ',
				rewriteValue: ' /v1 '
			}
		]);

		expect(normalized[0]?.rewriteMode).toBe('replace_prefix');
		expect(normalized[0]?.rewriteValue).toBe('/v1');
	});
});
