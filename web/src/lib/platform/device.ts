// Device facts and storage. Platform differences live in this folder only.

/** Reads a JSON value saved by `save` (undefined when missing or broken). */
export function load(key: string): unknown {
	try {
		const raw = localStorage.getItem(key);
		return raw === null ? undefined : (JSON.parse(raw) as unknown);
	} catch {
		return undefined;
	}
}

export function save(key: string, value: unknown): void {
	try {
		localStorage.setItem(key, JSON.stringify(value));
	} catch {
		// Private mode or full storage: settings just are not remembered.
	}
}

/** TVs with little memory (Chrome reports deviceMemory in GB). */
export function lowMemory(): boolean {
	const mem = (navigator as Navigator & { deviceMemory?: number }).deviceMemory;
	return mem !== undefined && mem <= 2;
}

export function prefersReducedMotion(): boolean {
	return matchMedia('(prefers-reduced-motion: reduce)').matches;
}

/** Whether the app runs as a TV app (Sprint 7 sets this per platform). */
export const isTV = false;

/** Leaves the app (TV only; the browser has nothing to exit). */
export function exitApp(): void {}
