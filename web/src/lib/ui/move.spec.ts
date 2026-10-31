import { describe, expect, it } from 'vitest';

import { moveIndex, rowStart, rowWindow, titleGradient } from './move';

describe('moveIndex', () => {
	it.each([
		// row of 10
		[0, 'left', 10, 10, null],
		[0, 'right', 10, 10, 1],
		[9, 'right', 10, 10, null],
		[3, 'up', 10, 10, null],
		[3, 'down', 10, 10, null],
		// grid of 7 items in rows of 3: 0 1 2 / 3 4 5 / 6
		[1, 'down', 7, 3, 4],
		[4, 'down', 7, 3, 6],
		[5, 'down', 7, 3, 6],
		[6, 'down', 7, 3, null],
		[2, 'right', 7, 3, null],
		[3, 'left', 7, 3, null],
		[4, 'up', 7, 3, 1],
		[1, 'up', 7, 3, null],
		[6, 'right', 7, 3, null],
		// list
		[0, 'down', 3, 1, 1],
		[2, 'down', 3, 1, null],
		[0, 'right', 3, 1, null],
	] as const)('%i %s (count %i, cols %i) → %s', (index, dir, count, cols, want) => {
		expect(moveIndex(index, dir, count, cols)).toBe(want);
	});
});

describe('rowStart', () => {
	it('scrolls only when the focus leaves the visible slots', () => {
		expect(rowStart(0, 3, 5, 20)).toBe(0);
		expect(rowStart(0, 5, 5, 20)).toBe(1);
		expect(rowStart(4, 2, 5, 20)).toBe(2);
		expect(rowStart(0, 19, 5, 20)).toBe(15);
		expect(rowStart(10, 3, 5, 4)).toBe(0);
	});
});

describe('rowWindow', () => {
	it('renders the visible slots plus a margin', () => {
		expect(rowWindow(0, 5, 100)).toEqual([0, 9]);
		expect(rowWindow(50, 5, 100)).toEqual([46, 59]);
		expect(rowWindow(98, 5, 100)).toEqual([94, 100]);
	});
});

describe('titleGradient', () => {
	it('is stable per title', () => {
		expect(titleGradient('Dark')).toBe(titleGradient('Dark'));
		expect(titleGradient('Dark')).not.toBe(titleGradient('Origem'));
		expect(titleGradient('Dark')).toMatch(/^linear-gradient\(135deg, hsl\(\d+, 45%, 28%\)/);
	});
});
