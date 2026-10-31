// LG webOS. Used by the TV shell only. Luna services are called through
// PalmServiceBridge directly (what LG's webOSTV.js does), so the package has
// no third-party library.

import type { Native } from './bridge';

/** The PalmServiceBridge every webOS web app has. */
export interface PalmBridge {
	onservicecallback?: (message: string) => void;
	call(uri: string, params: string): void;
	cancel?(): void;
}

export type PalmBridgeCtor = new () => PalmBridge;

const isCtor = (b: unknown): b is PalmBridgeCtor => typeof b === 'function';

export function palmBridge(w: object): PalmBridgeCtor | undefined {
	const b: unknown = Reflect.get(w, 'PalmServiceBridge');
	return isCtor(b) ? b : undefined;
}

/** One luna call; undefined when it fails or takes too long. */
export function luna(Bridge: PalmBridgeCtor, uri: string, timeoutMs = 2000): Promise<unknown> {
	return new Promise((resolve) => {
		let bridge: PalmBridge | undefined;
		const timer = setTimeout(() => {
			bridge?.cancel?.();
			resolve(undefined);
		}, timeoutMs);
		try {
			bridge = new Bridge();
			bridge.onservicecallback = (message) => {
				clearTimeout(timer);
				try {
					resolve(JSON.parse(message) as unknown);
				} catch {
					resolve(undefined);
				}
			};
			bridge.call(uri, '{}');
		} catch {
			clearTimeout(timer);
			resolve(undefined);
		}
	});
}

/** The IPv4 of a connectionmanager/getStatus answer (cable first). */
export function statusIp(status: unknown): string {
	if (typeof status !== 'object' || status === null) return '';
	for (const kind of ['wired', 'wifi']) {
		const net: unknown = Reflect.get(status, kind);
		if (typeof net !== 'object' || net === null) continue;
		const ip: unknown = Reflect.get(net, 'ipAddress');
		if (Reflect.get(net, 'state') === 'connected' && typeof ip === 'string' && ip !== '') return ip;
	}
	return '';
}

export function webosNative(Bridge: PalmBridgeCtor, close: () => void): Native {
	return {
		platform: 'webos',
		// webOS delivers every remote key; appinfo.json turns off its own Back handling.
		registerKeys() {},
		exit: close,
		async localIp() {
			return statusIp(await luna(Bridge, 'luna://com.palm.connectionmanager/getStatus'));
		},
	};
}
