import { describe, expect, it, vi } from 'vitest';

import { detectNative, isIPv4 } from './native';
import { TIZEN_KEYS, tizenNative, type TizenApi } from './tizen';
import { luna, statusIp, webosNative, type PalmBridge } from './webos';

/** A fake Tizen with the given network answers. */
function fakeTizen(
	ips: Record<string, string>,
): TizenApi & { registered: string[]; exited: boolean } {
	const t = {
		registered: [] as string[],
		exited: false,
		tvinputdevice: {
			registerKey(name: string) {
				if (name === 'ColorF3Blue') throw new Error('NotSupportedError');
				t.registered.push(name);
			},
		},
		application: { getCurrentApplication: () => ({ exit: () => void (t.exited = true) }) },
		systeminfo: {
			getPropertyValue(prop: string, ok: (v: unknown) => void, fail?: (e: unknown) => void) {
				const ip = ips[prop];
				if (ip === undefined) fail?.(new Error('NotSupportedError'));
				else ok({ ipAddress: ip });
			},
		},
	};
	return t;
}

/** A fake PalmServiceBridge answering every call with `answer`. */
function fakePalm(answer: string | undefined) {
	const calls: string[] = [];
	class Bridge implements PalmBridge {
		onservicecallback?: (m: string) => void;
		call(uri: string) {
			calls.push(uri);
			if (answer !== undefined) setTimeout(() => this.onservicecallback?.(answer), 0);
		}
	}
	return { Bridge, calls };
}

describe('isIPv4', () => {
	it.each([
		['192.168.0.10', true],
		['10.0.0.1', true],
		['0.0.0.0', false],
		['256.1.1.1', false],
		['192.168.0', false],
		['meupc.local', false],
		['', false],
	])('%s → %s', (ip, want) => {
		expect(isIPv4(ip)).toBe(want);
	});
});

describe('detectNative', () => {
	it('finds each platform by its API', () => {
		const bridge = { exit() {}, openSetup() {}, connect() {}, localIp: () => '' };
		expect(detectNative({ MediaserverAndroid: bridge }).platform).toBe('android');
		expect(detectNative({ tizen: fakeTizen({}) }).platform).toBe('tizen');
		expect(detectNative({ PalmServiceBridge: fakePalm(undefined).Bridge }).platform).toBe('webos');
		expect(detectNative({}).platform).toBe('browser');
	});

	it('android: keys, exit, IP and connect go to the bridge', async () => {
		const bridge = {
			exit: vi.fn<() => void>(),
			openSetup() {},
			connect: vi.fn<(url: string) => void>(),
			localIp: () => '192.168.1.20',
		};
		const n = detectNative({ MediaserverAndroid: bridge });
		n.exit();
		n.connect?.('http://192.168.1.5:8080/?tv=android');
		expect(bridge.exit).toHaveBeenCalled();
		expect(bridge.connect).toHaveBeenCalledWith('http://192.168.1.5:8080/?tv=android');
		await expect(n.localIp()).resolves.toBe('192.168.1.20');
	});
});

describe('tizen', () => {
	it('registers every key the remote has', () => {
		const t = fakeTizen({});
		tizenNative(t).registerKeys();
		expect(t.registered).toEqual(TIZEN_KEYS.filter((k) => k !== 'ColorF3Blue'));
	});

	it('exits through the application API', () => {
		const t = fakeTizen({});
		tizenNative(t).exit();
		expect(t.exited).toBe(true);
	});

	it.each([
		[{ ETHERNET_NETWORK: '192.168.0.7', WIFI_NETWORK: '192.168.0.8' }, '192.168.0.7'],
		[{ ETHERNET_NETWORK: '0.0.0.0', WIFI_NETWORK: '192.168.0.8' }, '192.168.0.8'],
		[{ WIFI_NETWORK: '10.0.0.3' }, '10.0.0.3'],
		[{}, ''],
	])('local IP of %o is %s', async (ips, want) => {
		await expect(tizenNative(fakeTizen(ips)).localIp()).resolves.toBe(want);
	});
});

describe('webos', () => {
	it('reads the IP from connectionmanager (cable first)', async () => {
		const status = {
			returnValue: true,
			wired: { state: 'connected', ipAddress: '192.168.0.9' },
			wifi: { state: 'connected', ipAddress: '192.168.0.10' },
		};
		const { Bridge, calls } = fakePalm(JSON.stringify(status));
		await expect(webosNative(Bridge, () => {}).localIp()).resolves.toBe('192.168.0.9');
		expect(calls).toEqual(['luna://com.palm.connectionmanager/getStatus']);
	});

	it.each([
		[
			{ wired: { state: 'disconnected' }, wifi: { state: 'connected', ipAddress: '10.1.1.4' } },
			'10.1.1.4',
		],
		[{ wired: { state: 'disconnected' } }, ''],
		[null, ''],
	])('statusIp(%o) → %s', (status, want) => {
		expect(statusIp(status)).toBe(want);
	});

	it('gives up on a luna call that never answers', async () => {
		vi.useFakeTimers();
		const answer = luna(fakePalm(undefined).Bridge, 'luna://x/y', 500);
		vi.advanceTimersByTime(500);
		await expect(answer).resolves.toBeUndefined();
		vi.useRealTimers();
	});

	it('exits with the given close function', () => {
		const close = vi.fn<() => void>();
		webosNative(fakePalm(undefined).Bridge, close).exit();
		expect(close).toHaveBeenCalled();
	});
});
