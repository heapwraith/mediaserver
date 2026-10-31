import { describe, expect, it } from 'vitest';

import { digitKey, keyAction } from './keys';

describe('keyAction', () => {
	it.each([
		[{ key: 'ArrowLeft', keyCode: 37 }, 'left'],
		[{ key: 'Enter', keyCode: 13 }, 'enter'],
		[{ key: 'Escape', keyCode: 27 }, 'back'],
		[{ key: 'Unidentified', keyCode: 10009 }, 'back'],
		[{ key: 'Unidentified', keyCode: 461 }, 'back'],
		[{ key: 'GoBack', keyCode: 0 }, 'back'],
		[{ key: ' ', keyCode: 32 }, 'playpause'],
		[{ key: 'a', keyCode: 65 }, null],
	])('%o → %s', (e, want) => {
		expect(keyAction(e)).toBe(want);
	});

	// Media and color keys: DOM names (Android TV, recent TVs) and the codes
	// Tizen and webOS send with key 'Unidentified'.
	it.each([
		[{ key: 'MediaPlayPause', keyCode: 179 }, 'playpause'],
		[{ key: 'Unidentified', keyCode: 10252 }, 'playpause'],
		[{ key: 'MediaPlay', keyCode: 415 }, 'play'],
		[{ key: 'Unidentified', keyCode: 415 }, 'play'],
		[{ key: 'MediaPause', keyCode: 19 }, 'pause'],
		[{ key: 'Unidentified', keyCode: 19 }, 'pause'],
		[{ key: 'MediaStop', keyCode: 413 }, 'stop'],
		[{ key: 'Unidentified', keyCode: 413 }, 'stop'],
		[{ key: 'MediaFastForward', keyCode: 417 }, 'forward'],
		[{ key: 'Unidentified', keyCode: 417 }, 'forward'],
		[{ key: 'MediaRewind', keyCode: 412 }, 'rewind'],
		[{ key: 'Unidentified', keyCode: 412 }, 'rewind'],
		[{ key: 'ColorF0Red', keyCode: 403 }, 'red'],
		[{ key: 'Unidentified', keyCode: 404 }, 'green'],
		[{ key: 'ColorF2Yellow', keyCode: 0 }, 'yellow'],
		[{ key: 'Unidentified', keyCode: 406 }, 'blue'],
	])('remote %o → %s', (e, want) => {
		expect(keyAction(e)).toBe(want);
	});
});

describe('digitKey', () => {
	it.each([
		[{ key: '7', keyCode: 55 }, 7],
		[{ key: 'Unidentified', keyCode: 48 }, 0],
		[{ key: 'Unidentified', keyCode: 105 }, 9],
		[{ key: 'a', keyCode: 65 }, null],
		[{ key: 'Enter', keyCode: 13 }, null],
	])('%o → %s', (e, want) => {
		expect(digitKey(e)).toBe(want);
	});
});
