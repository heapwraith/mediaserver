import { describe, expect, it } from 'vitest';

import type { Progress } from '../api/types';
import { fraction, nextCountdown, remaining, resumeAt, shouldSave } from './progress';

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
});
