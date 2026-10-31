import type { Subtitle } from '#lib/api/types.ts';

const primary = (language: string) => language.toLowerCase().split('-')[0];

/** Index in the supplied list, or -1 for subtitles off. */
export function defaultSubtitle(subs: Subtitle[], pref: string, audioLang = ''): number {
	if (pref === '') return -1;
	const matches = (s: Subtitle) =>
		(s.kind === 'text' || s.kind === 'ass') && primary(s.language ?? '') === primary(pref);
	if (primary(audioLang) === primary(pref)) {
		return subs.findIndex((s) => matches(s) && s.forced === true);
	}
	const regular = subs.findIndex((s) => matches(s) && s.forced !== true);
	return regular >= 0 ? regular : subs.findIndex(matches);
}

export interface ASSRenderer {
	setOffset(offset: number): void;
	destroy(): void;
}

/** WASM, worker and fallback font are fetched only when ASS is selected. */
export async function attachASS(
	video: HTMLVideoElement,
	url: string,
	fonts: string[],
	offset: number,
): Promise<ASSRenderer> {
	const { default: JASSUB } = await import('jassub');
	const renderer = new JASSUB({
		video,
		subUrl: url,
		fonts,
		timeOffset: offset,
		queryFonts: false,
	});
	try {
		await renderer.ready;
	} catch (error) {
		await renderer.destroy().catch(() => undefined);
		throw error;
	}
	return {
		setOffset(value) {
			renderer.timeOffset = value;
		},
		destroy() {
			void renderer.destroy().catch(() => undefined);
		},
	};
}
