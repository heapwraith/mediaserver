import type { Playback } from '#lib/api/types.ts';

/** A stream loaded in the <video>. */
export interface Source {
	/** Last media second ready to play (Infinity: the whole file). */
	end(): number;
	destroy(): void;
}

function unload(video: HTMLVideoElement): void {
	video.removeAttribute('src');
	video.load();
}

/**
 * Loads p in video: the file itself, or HLS through hls.js (MSE; loaded
 * only here, outside the initial bundle) or natively when there is no MSE.
 * onFatal: the HLS stream broke (e.g. its session was stopped).
 */
export async function attach(
	video: HTMLVideoElement,
	p: Playback,
	onFatal: () => void,
): Promise<Source> {
	if (p.direct) {
		video.src = p.url;
		return { end: () => Infinity, destroy: () => unload(video) };
	}
	const { default: Hls } = await import('hls.js/light');
	if (!Hls.isSupported()) {
		video.src = p.url;
		const s = video.seekable;
		return {
			end: () => p.start + (s.length > 0 ? s.end(s.length - 1) : 0),
			destroy: () => unload(video),
		};
	}
	// The session playlist grows while FFmpeg works (EVENT): start at its
	// beginning, not at the "live" edge. backBufferLength spares TV memory.
	const hls = new Hls({ startPosition: 0, backBufferLength: 90 });
	hls.on(Hls.Events.ERROR, (_event, data) => {
		if (data.fatal) onFatal();
	});
	hls.loadSource(p.url);
	hls.attachMedia(video);
	return {
		end: () => p.start + (hls.latestLevelDetails?.totalduration ?? 0),
		destroy: () => hls.destroy(),
	};
}
