import type { Health } from '$lib/design/mock';

// One mapping from state to colour + words. Colour is never the only signal (10-ui.md §2): every use shows the label
// or carries it for screen readers.
export const health: Record<Health, { label: string; dot: string; text: string; bar: string }> = {
	up: { label: 'Up', dot: 'bg-ok', text: 'text-ok-text', bar: 'bg-ok' },
	degraded: { label: 'Degraded', dot: 'bg-warn', text: 'text-warn-text', bar: 'bg-warn' },
	down: { label: 'Down', dot: 'bg-bad', text: 'text-bad-text', bar: 'bg-bad' },
	paused: { label: 'Paused', dot: 'bg-idle', text: 'text-muted-foreground', bar: 'bg-idle' },
	unknown: { label: 'Unknown', dot: 'bg-idle', text: 'text-muted-foreground', bar: 'bg-idle' }
};
