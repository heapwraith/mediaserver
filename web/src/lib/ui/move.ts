import type { Direction } from '../platform/keys';

/**
 * Next index when moving `dir` in a collection of `count` items laid out in
 * rows of `cols` (a single row: cols = count; a list: cols = 1). Returns
 * null at an edge, so the move goes on to the next element on screen.
 */
export function moveIndex(
	index: number,
	dir: Direction,
	count: number,
	cols: number,
): number | null {
	const c = Math.max(1, cols);
	if (dir === 'left') return index % c === 0 ? null : index - 1;
	if (dir === 'right') return index % c === c - 1 || index + 1 >= count ? null : index + 1;
	if (dir === 'up') return index - c < 0 ? null : index - c;
	if (index + c < count) return index + c;
	// Down into the last, incomplete row: go to its last item.
	return Math.floor(index / c) < Math.floor((count - 1) / c) ? count - 1 : null;
}

/**
 * First visible item of a horizontal row after focusing `index`: the row
 * only scrolls when the focus would leave the `visible` slots.
 */
export function rowStart(start: number, index: number, visible: number, count: number): number {
	let s = start;
	if (index < s) s = index;
	if (index >= s + visible) s = index - visible + 1;
	return Math.max(0, Math.min(s, Math.max(0, count - visible)));
}

/** Items rendered by a virtualized row: the visible slots plus a margin. */
export function rowWindow(
	start: number,
	visible: number,
	count: number,
	margin = 4,
): [number, number] {
	return [Math.max(0, start - margin), Math.min(count, start + visible + margin)];
}

/** Background for a card without image: a stable gradient from the title. */
export function titleGradient(title: string): string {
	let h = 0;
	for (let i = 0; i < title.length; i++) h = (h * 31 + title.charCodeAt(i)) >>> 0;
	const hue = h % 360;
	return `linear-gradient(135deg, hsl(${hue}, 45%, 28%), hsl(${(hue + 40) % 360}, 50%, 14%))`;
}
