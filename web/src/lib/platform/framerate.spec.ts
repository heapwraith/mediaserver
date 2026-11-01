import { describe, expect, it } from 'vitest';

import { bestMode, parseDisplayModes, type DisplayModes } from './framerate';

const FILM = 24000 / 1001;
const NTSC = 30000 / 1001;

const screen = (...rates: number[]): DisplayModes => ({
	current: 1,
	modes: rates.map((rate, i) => ({ id: i + 1, width: 1920, height: 1080, rate })),
});

describe('bestMode', () => {
	it('stays when the screen has no multiple of the video', () => {
		expect(bestMode(screen(60), FILM)).toBe(0);
		expect(bestMode(screen(60, 50), FILM)).toBe(0);
	});
	it('picks the exact rate', () => {
		expect(bestMode(screen(60, 24, FILM), FILM)).toBe(3);
		expect(bestMode(screen(60, 59.94, 50), NTSC)).toBe(2);
		expect(bestMode(screen(60, 50), 25)).toBe(2);
	});
	it('accepts a nearly exact rate', () => {
		expect(bestMode(screen(60, 24), FILM)).toBe(2);
		expect(bestMode(screen(60), NTSC)).toBe(1);
	});
	it('prefers the lowest of equal rates', () => {
		expect(bestMode(screen(120, 24), 24)).toBe(2);
	});
	it('does not change the resolution', () => {
		const d: DisplayModes = {
			current: 1,
			modes: [
				{ id: 1, width: 1920, height: 1080, rate: 60 },
				{ id: 2, width: 3840, height: 2160, rate: 24 },
			],
		};
		expect(bestMode(d, 24)).toBe(0);
	});
	it('ignores an unknown frame rate or mode', () => {
		expect(bestMode(screen(24), 0)).toBe(0);
		expect(bestMode({ current: 9, modes: screen(24).modes }, 24)).toBe(0);
	});
	it('never picks a rate below the video', () => {
		expect(bestMode(screen(30), 60)).toBe(0);
	});
});

describe('parseDisplayModes', () => {
	it('reads the JSON of the Android bridge', () => {
		const raw = '{"current":1,"modes":[{"id":1,"width":1920,"height":1080,"rate":60.0}]}';
		expect(parseDisplayModes(raw)?.modes[0]?.rate).toBe(60);
	});
	it('refuses anything else', () => {
		for (const raw of ['', 'x', 'null', '{"current":1}', '{"current":1,"modes":[{"id":1}]}']) {
			expect(parseDisplayModes(raw)).toBeUndefined();
		}
	});
});
