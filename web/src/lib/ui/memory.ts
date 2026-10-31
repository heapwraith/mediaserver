// Position of each row/grid, kept while the app runs so coming back to a
// page restores where the user was (AGENTS.md §7).
const positions = new Map<string, { index: number; start: number }>();

const key = (id: string) => `${location.pathname}#${id}`;

export function recall(id: string): { index: number; start: number } {
	return positions.get(key(id)) ?? { index: 0, start: 0 };
}

export function remember(id: string, index: number, start = 0): void {
	positions.set(key(id), { index, start });
}
