// What this device plays, sent to the server so it decides between direct
// play, remux and transcoding (internal/stream/plan.go). Feature detection
// only, never the user agent (AGENTS.md §5.1).

import { lowMemory } from './device';
import { extraCapabilities } from './platform';

/** Containers of direct play, asked to the <video> element. */
const containers: Record<string, string> = {
	mp4: 'video/mp4',
	webm: 'video/webm',
	mkv: 'video/x-matroska',
	mov: 'video/quicktime',
};

/** The container tokens (a direct play that failed retries without them). */
export const CONTAINERS = Object.keys(containers);

/** Codecs in fMP4 (what HLS carries), asked to MediaSource. */
const codecs: Record<string, string> = {
	h264: 'video/mp4; codecs="avc1.640028"',
	'h264:10': 'video/mp4; codecs="avc1.6E0028"',
	hevc: 'video/mp4; codecs="hvc1.1.6.L120.90"',
	'hevc:10': 'video/mp4; codecs="hvc1.2.4.L153.90"',
	av1: 'video/mp4; codecs="av01.0.08M.08"',
	'av1:10': 'video/mp4; codecs="av01.0.08M.10"',
	vp9: 'video/mp4; codecs="vp09.00.40.08"',
	'vp9:10': 'video/mp4; codecs="vp09.02.40.10"',
	aac: 'audio/mp4; codecs="mp4a.40.2"',
	mp3: 'audio/mp4; codecs="mp4a.6B"',
	opus: 'audio/mp4; codecs="opus"',
	flac: 'audio/mp4; codecs="flac"',
	ac3: 'audio/mp4; codecs="ac-3"',
	eac3: 'audio/mp4; codecs="ec-3"',
};

type Check = (type: string) => boolean;

/** The tokens of internal/stream/plan.go this device supports. */
export function detectCapabilities(canPlay: Check, mse: Check | undefined): string[] {
	const codec = mse ?? canPlay;
	return [
		...Object.keys(containers).filter((k) => canPlay(containers[k] ?? '')),
		...Object.keys(codecs).filter((k) => codec(codecs[k] ?? '')),
	];
}

let cached: string[] | undefined;

/** Old TVs ignore WorkerOptions.type; only module workers read its getter. */
export function supportsASS(): boolean {
	if (
		lowMemory() ||
		typeof Worker === 'undefined' ||
		typeof OffscreenCanvas === 'undefined' ||
		typeof WebAssembly === 'undefined'
	)
		return false;
	let supported = false;
	const url = URL.createObjectURL(new Blob([''], { type: 'text/javascript' }));
	try {
		const worker = new Worker(url, {
			get type(): WorkerType {
				supported = true;
				return 'module';
			},
		});
		worker.terminate();
	} catch {
		return false;
	} finally {
		URL.revokeObjectURL(url);
	}
	return supported;
}

export function capabilities(): string[] {
	if (cached === undefined) {
		const video = document.createElement('video');
		const ms = (window as Window & { ManagedMediaSource?: typeof MediaSource }).ManagedMediaSource;
		const MS = typeof MediaSource === 'undefined' ? ms : MediaSource;
		cached = detectCapabilities(
			(t) => video.canPlayType(t) !== '',
			MS === undefined ? undefined : (t) => MS.isTypeSupported(t),
		);
		for (const c of extraCapabilities()) if (!cached.includes(c)) cached.push(c);
		if (supportsASS()) cached.push('ass');
	}
	return cached;
}
