import { describe, expect, it } from 'vitest';

import { clock } from './time';

describe('clock', () => {
	it.each([
		[0, '0:00'],
		[7.9, '0:07'],
		[754, '12:34'],
		[3723, '1:02:03'],
		[Number.NaN, '0:00'],
		[-5, '0:00'],
	])('%d → %s', (s, want) => {
		expect(clock(s)).toBe(want);
	});
});
