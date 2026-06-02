import { beforeEach, describe, expect, it } from 'vitest';
import {
	DEFAULT_WIDGETS,
	loadDashboardWidgetPrefs,
	loadWidgetPrefs,
	parseWidgetPrefsJson,
	saveWidgetPrefs,
	type WidgetPrefs
} from './widgets';

const STORAGE_KEY = 'hashi.overview.widgets';

function prefs(overrides: Partial<WidgetPrefs>): WidgetPrefs {
	return {
		enabled: Object.fromEntries(DEFAULT_WIDGETS.map((w) => [w.id, true])),
		order: DEFAULT_WIDGETS.map((w) => w.id),
		...overrides
	};
}

describe('overview widget preferences', () => {
	beforeEach(() => {
		localStorage.clear();
	});

	it('parses persisted settings without treating local storage as canonical', () => {
		saveWidgetPrefs(prefs({ enabled: { 'resource-health': false } }));

		const parsed = parseWidgetPrefsJson(null);

		expect(parsed.enabled['resource-health']).toBe(true);
	});

	it('loads valid dashboard settings first and caches them locally', () => {
		saveWidgetPrefs(prefs({ enabled: { 'dns-sync': true } }));

		const loaded = loadDashboardWidgetPrefs({
			overviewWidgetsJson: JSON.stringify({
				enabled: { 'dns-sync': false },
				order: ['dns-sync', 'resource-health']
			})
		});

		expect(loaded.enabled['dns-sync']).toBe(false);
		expect(loaded.order.slice(0, 2)).toEqual(['dns-sync', 'resource-health']);
		expect(JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}').enabled['dns-sync']).toBe(false);
	});

	it('falls back to the local cache when persisted settings are missing or invalid', () => {
		saveWidgetPrefs(prefs({ enabled: { 'security-events': false } }));

		expect(loadDashboardWidgetPrefs({ overviewWidgetsJson: null }).enabled['security-events']).toBe(
			false
		);
		expect(loadDashboardWidgetPrefs({ overviewWidgetsJson: '{nope' }).enabled['security-events']).toBe(
			false
		);
		expect(loadWidgetPrefs().enabled['security-events']).toBe(false);
	});
});
