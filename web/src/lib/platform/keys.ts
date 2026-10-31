// Remote/keyboard keys → app actions. The only place that knows key codes
// (AGENTS.md §5.2): TV codes are added here in Sprint 7.

export type Direction = 'up' | 'down' | 'left' | 'right';
export type Action = Direction | 'enter' | 'back' | 'playpause';

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
	MediaPlayPause: 'playpause',
	' ': 'playpause',
};

// Older TV browsers send only keyCode.
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
};

/** The action of a key event, or null when the app does not use the key. */
export function keyAction(e: Pick<KeyboardEvent, 'key' | 'keyCode'>): Action | null {
	return byKey[e.key] ?? byCode[e.keyCode] ?? null;
}

export const isDirection = (a: Action): a is Direction =>
	a === 'up' || a === 'down' || a === 'left' || a === 'right';
