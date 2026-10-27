// WCAG 2.x relative luminance and contrast ratio, for the custom theme editor's save guard (10-ui.md §2).
function channel(c: number) {
	const s = c / 255;
	return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
}
export function luminance(hex: string): number {
	const m = /^#?([0-9a-f]{6})$/i.exec(hex.trim());
	if (!m) return NaN;
	const n = parseInt(m[1]!, 16);
	return (
		0.2126 * channel((n >> 16) & 255) + 0.7152 * channel((n >> 8) & 255) + 0.0722 * channel(n & 255)
	);
}
export function contrast(a: string, b: string): number {
	const [x, y] = [luminance(a), luminance(b)].sort((p, q) => q - p) as [number, number];
	return (x + 0.05) / (y + 0.05);
}
