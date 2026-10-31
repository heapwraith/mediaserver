import { describe, expect, it } from 'vitest';

import {
	androidBridge,
	appMessage,
	isAppMessage,
	isShellMessage,
	keyMessage,
	type Platform,
} from './bridge';
import { detectPlatform, extraCapabilities } from './platform';

describe('detectPlatform', () => {
	const base: Parameters<typeof detectPlatform>[0] = {
		search: '',
		stored: null,
		framed: false,
		android: false,
	};
	it.each<[string, Partial<typeof base>, Platform]>([
		['plain browser', {}, 'browser'],
		['tizen shell', { search: '?tv=tizen', framed: true }, 'tizen'],
		['webos shell', { search: '?x=1&tv=webos', framed: true }, 'webos'],
		['reload keeps the platform', { stored: 'tizen', framed: true }, 'tizen'],
		// A tab opened with ?tv=tizen has no shell to talk to.
		['not framed', { search: '?tv=tizen' }, 'browser'],
		['unknown value', { search: '?tv=roku', framed: true }, 'browser'],
		['android bridge wins', { search: '?tv=android', android: true }, 'android'],
		['android without bridge', { search: '?tv=android' }, 'browser'],
		['param wins over stored', { search: '?tv=webos', stored: 'tizen', framed: true }, 'webos'],
	])('%s', (_name, env, want) => {
		expect(detectPlatform({ ...base, ...env })).toBe(want);
	});
});

describe('extraCapabilities', () => {
	it('adds the containers and audio Samsung and LG play natively', () => {
		expect(extraCapabilities('tizen')).toEqual(['mkv', 'ac3', 'eac3']);
		expect(extraCapabilities('webos')).toEqual(['mkv', 'ac3', 'eac3']);
	});
	it('trusts detection elsewhere', () => {
		expect(extraCapabilities('android')).toEqual([]);
		expect(extraCapabilities('browser')).toEqual([]);
	});
});

describe('bridge messages', () => {
	it('accepts its own messages', () => {
		expect(isAppMessage(appMessage('exit'))).toBe(true);
		expect(isShellMessage(keyMessage('ColorF0Red', 403))).toBe(true);
	});
	it.each([
		null,
		'exit',
		{ from: 'other', type: 'exit' },
		{ from: 'mediaserver-app', type: 'rm -rf' },
		{ from: 'mediaserver-shell', type: 'key', key: 1, keyCode: 403 },
	])('rejects %o', (data) => {
		expect(isAppMessage(data) || isShellMessage(data)).toBe(false);
	});
	it('finds the Android bridge only when it has the methods', () => {
		const bridge = { exit() {}, openSetup() {}, connect() {}, localIp: () => '' };
		expect(androidBridge({ MediaserverAndroid: bridge })).toBe(bridge);
		expect(androidBridge({ MediaserverAndroid: {} })).toBeUndefined();
		expect(androidBridge({})).toBeUndefined();
	});
});
