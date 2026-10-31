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

// Fullscreen (Safari 15 still needs the webkit-prefixed API).
type WebkitDocument = Document & {
	webkitFullscreenElement?: Element | null;
	webkitExitFullscreen?: () => void;
};
type WebkitElement = HTMLElement & { webkitRequestFullscreen?: () => void };

export function isFullscreen(): boolean {
	const d = document as WebkitDocument;
	return (d.fullscreenElement ?? d.webkitFullscreenElement ?? null) !== null;
}

/** Asks for fullscreen; the browser may refuse without a recent user gesture. */
export async function enterFullscreen(el: HTMLElement): Promise<void> {
	if (isTV || isFullscreen()) return; // TV apps already fill the screen
	const w = el as WebkitElement;
	try {
		if (typeof el.requestFullscreen === 'function') await el.requestFullscreen();
		else w.webkitRequestFullscreen?.();
	} catch {
		// Refused (no user gesture): the button is still there.
	}
}

export async function exitFullscreen(): Promise<void> {
	if (!isFullscreen()) return;
	const d = document as WebkitDocument;
	try {
		if (typeof document.exitFullscreen === 'function') await document.exitFullscreen();
		else d.webkitExitFullscreen?.();
	} catch {
		// Already left.
	}
}
