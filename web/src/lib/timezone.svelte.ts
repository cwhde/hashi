// The General settings' time display (09 §2) as reactive state: times already on screen re-render when it loads or
// changes. undefined is the browser's own zone.
export const tz = $state<{ zone: 'UTC' | undefined }>({ zone: undefined });
