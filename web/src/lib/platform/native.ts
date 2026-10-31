// What the TV shell asks the device itself: which TV it is, the remote keys
// to enable, the local IP (to look for servers) and how to leave the app.
// Only the shell (web/src/shell) uses this; the app runs on the server's
// origin and asks the shell instead (bridge.ts).

import { androidBridge, type AndroidBridge, type Native } from './bridge';
import { tizenApi, tizenNative } from './tizen';
import { palmBridge, webosNative } from './webos';

export type { Native } from './bridge';

const IPV4 = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/;

export function isIPv4(s: string): boolean {
	const m = IPV4.exec(s);
	return m !== null && m.slice(1).every((n) => Number(n) <= 255) && s !== '0.0.0.0';
}

export function androidNative(b: AndroidBridge): Native {
	return {
		platform: 'android',
		registerKeys() {}, // the wrapper forwards Back, media and color keys itself
		exit: () => b.exit(),
		localIp: async () => {
			const ip = b.localIp();
			return isIPv4(ip) ? ip : '';
		},
		connect: (url) => b.connect(url),
	};
}

export const browserNative: Native = {
	platform: 'browser',
	registerKeys() {},
	exit() {},
	localIp: async () => '',
};

/** The device the shell runs on, by the APIs it has (never the user agent). */
export function detectNative(w: object): Native {
	const android = androidBridge(w);
	if (android !== undefined) return androidNative(android);
	const tizen = tizenApi(w);
	if (tizen !== undefined) return tizenNative(tizen);
	const palm = palmBridge(w);
	if (palm !== undefined) return webosNative(palm, () => window.close());
	return browserNative;
}
