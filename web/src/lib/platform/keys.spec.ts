import { describe, expect, it } from 'vitest';

import { keyAction } from './keys';

describe('keyAction', () => {
	it.each([
		[{ key: 'ArrowLeft', keyCode: 37 }, 'left'],
		[{ key: 'Enter', keyCode: 13 }, 'enter'],
		[{ key: 'Escape', keyCode: 27 }, 'back'],
		[{ key: 'Unidentified', keyCode: 10009 }, 'back'],
		[{ key: 'Unidentified', keyCode: 461 }, 'back'],
		[{ key: ' ', keyCode: 32 }, 'playpause'],
		[{ key: 'a', keyCode: 65 }, null],
	])('%o → %s', (e, want) => {
		expect(keyAction(e)).toBe(want);
	});
});
