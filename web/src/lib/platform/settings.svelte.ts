import { load, lowMemory, prefersReducedMotion, save } from './device';

const KEY = 'mediaserver.settings';

export const SUBTITLE_SIZES = ['s', 'm', 'l', 'xl'] as const;
export const SUBTITLE_BACKGROUNDS = ['none', 'soft', 'solid'] as const;
export type SubtitleSize = (typeof SUBTITLE_SIZES)[number];
export type SubtitleBackground = (typeof SUBTITLE_BACKGROUNDS)[number];

interface Settings {
	/** Turns off every animation. */
	reduceMotion: boolean;
	/** Muted trailer looping behind the details page. */
	backgroundTrailer: boolean;
	/** The next episode starts by itself after a countdown. */
	autoplayNext: boolean;
	/** Text subtitles (SRT, WebVTT): ASS keeps the style of its file. */
	subtitleSize: SubtitleSize;
	subtitleBackground: SubtitleBackground;
}

/** The saved value when it is one of `allowed`. */
function saved<T extends string>(from: object, key: string, allowed: readonly T[], fallback: T): T {
	const v: unknown = Reflect.get(from, key);
	return allowed.find((a) => a === v) ?? fallback;
}

function initial(): Settings {
	const weak = lowMemory();
	const s: Settings = {
		reduceMotion: weak || prefersReducedMotion(),
		backgroundTrailer: !weak,
		autoplayNext: true,
		subtitleSize: 'm',
		subtitleBackground: 'soft',
	};
	const stored = load(KEY);
	if (typeof stored === 'object' && stored !== null) {
		for (const k of ['reduceMotion', 'backgroundTrailer', 'autoplayNext'] as const) {
			const v: unknown = k in stored ? Reflect.get(stored, k) : undefined;
			if (typeof v === 'boolean') s[k] = v;
		}
		s.subtitleSize = saved(stored, 'subtitleSize', SUBTITLE_SIZES, s.subtitleSize);
		s.subtitleBackground = saved(
			stored,
			'subtitleBackground',
			SUBTITLE_BACKGROUNDS,
			s.subtitleBackground,
		);
	}
	return s;
}

/** User settings, saved on the device. */
export const settings: Settings = $state(initial());

export function saveSettings(): void {
	save(KEY, $state.snapshot(settings));
}
