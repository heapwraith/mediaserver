// Remote/keyboard keys → app actions. The only place that knows key codes
// (AGENTS.md §5.2).

export type Direction = 'up' | 'down' | 'left' | 'right';
/** Media keys of TV remotes (and of PC keyboards that have them). */
export type MediaAction = 'playpause' | 'play' | 'pause' | 'stop' | 'forward' | 'rewind';
/** The four color keys of TV remotes. */
export type ColorAction = 'red' | 'green' | 'yellow' | 'blue';
export type Action = Direction | 'enter' | 'back' | MediaAction | ColorAction;

const byKey: Record<string, Action> = {
	ArrowUp: 'up',
	ArrowDown: 'down',
	ArrowLeft: 'left',
	ArrowRight: 'right',
	Enter: 'enter',
	Escape: 'back',
	Backspace: 'back',
	BrowserBack: 'back',
	GoBack: 'back',
	XF86Back: 'back',
	MediaPlayPause: 'playpause',
	' ': 'playpause',
	MediaPlay: 'play',
	MediaPause: 'pause',
	MediaStop: 'stop',
	MediaFastForward: 'forward',
	MediaRewind: 'rewind',
	ColorF0Red: 'red',
	ColorF1Green: 'green',
	ColorF2Yellow: 'yellow',
	ColorF3Blue: 'blue',
};

// TV browsers often send only keyCode (key 'Unidentified').
const byCode: Record<number, Action> = {
	37: 'left',
	38: 'up',
	39: 'right',
	40: 'down',
	13: 'enter',
	27: 'back',
	8: 'back',
	10009: 'back', // Samsung Tizen
	461: 'back', // LG webOS
	179: 'playpause',
	10252: 'playpause', // Tizen
	415: 'play',
	19: 'pause',
	413: 'stop',
	417: 'forward',
	412: 'rewind',
	403: 'red',
	404: 'green',
	405: 'yellow',
	406: 'blue',
};

/** The action of a key event, or null when the app does not use the key. */
export function keyAction(e: Pick<KeyboardEvent, 'key' | 'keyCode'>): Action | null {
	return byKey[e.key] ?? byCode[e.keyCode] ?? null;
}

export const isDirection = (a: Action): a is Direction =>
	a === 'up' || a === 'down' || a === 'left' || a === 'right';

/** The digit of a key event (keyboard row, numpad or remote number keys), or null. */
export function digitKey(e: Pick<KeyboardEvent, 'key' | 'keyCode'>): number | null {
	if (e.key.length === 1 && e.key >= '0' && e.key <= '9') return Number(e.key);
	if (e.keyCode >= 48 && e.keyCode <= 57) return e.keyCode - 48;
	if (e.keyCode >= 96 && e.keyCode <= 105) return e.keyCode - 96;
	return null;
}

/**
 * Sends a key to the page as if it came from the device: the TV shells use
 * it for keys their runtime does not deliver to the page by itself (Android
 * Back and media keys, keys pressed while the frame had no focus).
 */
export function dispatchKey(key: string, keyCode: number): void {
	const target = document.activeElement ?? document.body;
	const e = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true });
	// keyCode is not part of KeyboardEventInit; old TVs only send it.
	Object.defineProperty(e, 'keyCode', { get: () => keyCode });
	target.dispatchEvent(e);
}
