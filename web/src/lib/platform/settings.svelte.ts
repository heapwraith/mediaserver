import { load, lowMemory, prefersReducedMotion, save } from './device';

const KEY = 'mediaserver.settings';

interface Settings {
	/** Turns off every animation. */
	reduceMotion: boolean;
	/** Muted trailer looping behind the details page. */
	backgroundTrailer: boolean;
	/** The next episode starts by itself after a countdown. */
	autoplayNext: boolean;
}

function initial(): Settings {
	const weak = lowMemory();
	const s: Settings = {
		reduceMotion: weak || prefersReducedMotion(),
		backgroundTrailer: !weak,
		autoplayNext: true,
	};
	const saved = load(KEY);
	if (typeof saved === 'object' && saved !== null) {
		for (const k of ['reduceMotion', 'backgroundTrailer', 'autoplayNext'] as const) {
			const v: unknown = k in saved ? Reflect.get(saved, k) : undefined;
			if (typeof v === 'boolean') s[k] = v;
		}
	}
	return s;
}

/** User settings, saved on the device. */
export const settings: Settings = $state(initial());

export function saveSettings(): void {
	save(KEY, $state.snapshot(settings));
}
