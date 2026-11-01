// Which screen mode suits a video (the Android app only lists and applies them:
// tv/android MainActivity, docs/adr/008).

export interface DisplayMode {
	id: number;
	width: number;
	height: number;
	rate: number;
}

export interface DisplayModes {
	/** Id of the mode in use. */
	current: number;
	modes: DisplayMode[];
}

/** 29.97 fps on a 60 Hz screen repeats a frame only every ~33 s: still a match. */
const TOLERANCE = 0.005;

const isMode = (m: unknown): m is DisplayMode =>
	typeof m === 'object' &&
	m !== null &&
	['id', 'width', 'height', 'rate'].every((k) => typeof Reflect.get(m, k) === 'number');

/** The JSON of the Android bridge; undefined when it is empty or broken. */
export function parseDisplayModes(raw: string): DisplayModes | undefined {
	try {
		const d: unknown = JSON.parse(raw);
		if (typeof d !== 'object' || d === null) return undefined;
		const current: unknown = Reflect.get(d, 'current');
		const modes: unknown = Reflect.get(d, 'modes');
		if (typeof current !== 'number' || !Array.isArray(modes) || !modes.every(isMode))
			return undefined;
		return { current, modes };
	} catch {
		return undefined;
	}
}

/**
 * The mode at the current resolution whose rate is closest to a whole multiple
 * of fps (the lowest such rate on a tie); 0 when none is, and the screen stays.
 */
export function bestMode({ current, modes }: DisplayModes, fps: number): number {
	const now = modes.find((m) => m.id === current);
	if (now === undefined || !(fps > 0)) return 0;
	const error = (m: DisplayMode) =>
		Math.abs(m.rate / (fps * Math.max(1, Math.round(m.rate / fps))) - 1);
	const fits = modes
		.filter(
			(m) => m.width === now.width && m.height === now.height && m.rate >= fps * (1 - TOLERANCE),
		)
		.filter((m) => error(m) <= TOLERANCE)
		.sort((a, b) => error(a) - error(b) || a.rate - b.rate);
	return fits[0]?.id ?? 0;
}
