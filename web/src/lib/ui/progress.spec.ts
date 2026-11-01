import { describe, expect, it } from 'vitest';

import type { Progress } from '../api/types';
import {
	creditsAt,
	fraction,
	nextCountdown,
	remaining,
	resumeAt,
	shouldSave,
	skipIntro,
} from './progress';

const p = (position_ms: number, duration_ms = 600_000, watched = false): Progress => ({
	media_id: 'm',
	position_ms,
	duration_ms,
	watched,
});

describe('resumeAt', () => {
	it('resumes only started, unfinished items', () => {
		expect(resumeAt(undefined)).toBe(0);
		expect(resumeAt(p(29_000))).toBe(0);
		expect(resumeAt(p(120_000))).toBe(120);
		expect(resumeAt(p(590_000, 600_000, true))).toBe(0);
	});
});

describe('fraction', () => {
	it('is the share watched, hidden when watched or empty', () => {
		expect(fraction(p(150_000))).toBe(0.25);
		expect(fraction(p(0))).toBeUndefined();
		expect(fraction(p(10, 0))).toBeUndefined();
		expect(fraction(p(500_000, 600_000, true))).toBeUndefined();
		expect(fraction(p(700_000))).toBe(1);
	});
});

describe('remaining', () => {
	it('rounds to minutes, never 0', () => {
		expect(remaining(p(0, 32 * 60_000))).toBe('faltam 32 min');
		expect(remaining(p(599_000))).toBe('falta 1 min');
		expect(remaining(p(0, 95 * 60_000))).toBe('faltam 1 h 35 min');
	});
});

describe('shouldSave', () => {
	it('waits 10 s between saves and skips the first seconds', () => {
		expect(shouldSave(3, 0, 60_000)).toBe(false);
		expect(shouldSave(20, 50_000, 55_000)).toBe(false);
		expect(shouldSave(20, 45_000, 55_000)).toBe(true);
	});
});

describe('nextCountdown', () => {
	it('counts the last 15 s of long enough videos', () => {
		expect(nextCountdown(100, 1200)).toBeNull();
		expect(nextCountdown(1185.5, 1200)).toBe(15);
		expect(nextCountdown(1199.2, 1200)).toBe(1);
		expect(nextCountdown(1200, 1200)).toBe(0);
		expect(nextCountdown(20, 25)).toBeNull();
	});

	it('starts with the end credits when they are known', () => {
		expect(nextCountdown(1099, 1200, 1100)).toBeNull();
		expect(nextCountdown(1100, 1200, 1100)).toBe(15);
		expect(nextCountdown(1110.5, 1200, 1100)).toBe(5);
		expect(nextCountdown(1130, 1200, 1100)).toBe(0);
	});
});

describe('skipIntro and creditsAt', () => {
	const markers = [
		{ kind: 'intro' as const, start_ms: 60_000, end_ms: 100_000 },
		{ kind: 'credits' as const, start_ms: 1_100_000, end_ms: 1_200_000 },
	];
	it('offers the end of the opening while it plays', () => {
		expect(skipIntro(59, markers)).toBeNull();
		expect(skipIntro(60, markers)).toBe(100);
		expect(skipIntro(97.5, markers)).toBe(100);
		expect(skipIntro(98, markers)).toBeNull(); // the last 2 s: nothing left to skip
		expect(skipIntro(70, [])).toBeNull();
		expect(skipIntro(70)).toBeNull();
	});
	it('finds the credits', () => {
		expect(creditsAt(markers)).toBe(1100);
		expect(creditsAt([])).toBeUndefined();
	});
});
