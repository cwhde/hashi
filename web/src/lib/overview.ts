// The Overview start page's widgets (10-ui.md §3, bug 31). The order and which ones show are kept in Appearance as
// `overview_widgets`; a widget missing there (new in a later version) is added at the end and shown.
import type { Schemas } from '#lib/api/client.ts';

export type WidgetId = Schemas['OverviewWidget']['id'];
export type Widget = Schemas['OverviewWidget'];

export const widgetNames: Record<WidgetId, string> = {
	changes: 'Changes waiting',
	attention: 'Needs attention',
	health: 'Service health',
	edge: 'Edge hosts',
	security: 'Security, last 24 h',
	certificates: 'Certificates',
	activity: 'Recent activity'
};

export const widgetIds = Object.keys(widgetNames) as WidgetId[];

/** The stored list completed with every known widget, in the stored order. */
export function arrange(stored: Widget[] | undefined): Widget[] {
	const known = (stored ?? []).filter((w) => widgetIds.includes(w.id));
	const missing = widgetIds.filter((id) => !known.some((w) => w.id === id));
	return [...known, ...missing.map((id) => ({ id, visible: true }))];
}
