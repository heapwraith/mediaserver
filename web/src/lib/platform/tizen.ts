// Samsung Tizen (TV Web Device API). Used by the TV shell only.

import type { Native } from './bridge';

/**
 * Keys a Tizen app receives only after registering them (arrows, OK and Back
 * always arrive). Digits are for the PIN pad.
 */
export const TIZEN_KEYS = [
	'MediaPlayPause',
	'MediaPlay',
	'MediaPause',
	'MediaStop',
	'MediaFastForward',
	'MediaRewind',
	'ColorF0Red',
	'ColorF1Green',
	'ColorF2Yellow',
	'ColorF3Blue',
	'0',
	'1',
	'2',
	'3',
	'4',
	'5',
	'6',
	'7',
	'8',
	'9',
];

type Callback = (value: unknown) => void;

/** The parts of the global `tizen` object the shell uses. */
export interface TizenApi {
	tvinputdevice?: { registerKey(name: string): void };
	application?: { getCurrentApplication(): { exit(): void } };
	systeminfo?: {
		getPropertyValue(property: string, onSuccess: Callback, onError?: Callback): void;
	};
}

export function tizenApi(w: object): TizenApi | undefined {
	const t: unknown = Reflect.get(w, 'tizen');
	return typeof t === 'object' && t !== null ? t : undefined;
}

/** Reads one systeminfo network property; '' when missing or slow. */
function networkIp(t: TizenApi, property: string): Promise<string> {
	return new Promise((resolve) => {
		const timer = setTimeout(() => resolve(''), 1500);
		const done = (v: unknown) => {
			clearTimeout(timer);
			const ip = typeof v === 'object' && v !== null ? Reflect.get(v, 'ipAddress') : undefined;
			resolve(typeof ip === 'string' ? ip : '');
		};
		try {
			t.systeminfo?.getPropertyValue(property, done, () => done(undefined));
			if (t.systeminfo === undefined) done(undefined);
		} catch {
			done(undefined);
		}
	});
}

export function tizenNative(t: TizenApi): Native {
	return {
		platform: 'tizen',
		registerKeys() {
			for (const key of TIZEN_KEYS) {
				try {
					t.tvinputdevice?.registerKey(key);
				} catch {
					// This TV's remote has no such key.
				}
			}
		},
		exit() {
			try {
				t.application?.getCurrentApplication().exit();
			} catch {
				// Nothing else to do: the Home key still leaves.
			}
		},
		async localIp() {
			// Cable first: a TV with both usually streams over the cable.
			const ips = await Promise.all([
				networkIp(t, 'ETHERNET_NETWORK'),
				networkIp(t, 'WIFI_NETWORK'),
			]);
			return ips.find((ip) => /^\d+\.\d+\.\d+\.\d+$/.test(ip) && ip !== '0.0.0.0') ?? '';
		},
	};
}
