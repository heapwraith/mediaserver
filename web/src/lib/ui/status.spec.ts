import { describe, expect, it } from 'vitest';

import { bytes, duration, encoderName } from './status';

describe('status formatting', () => {
	it.each([
		[0, '0 B'],
		[512, '512 B'],
		[1536, '1,5 KB'],
		[52_428_800, '50 MB'],
		[10_737_418_240, '10 GB'],
	])('bytes(%d) = %s', (n, want) => {
		expect(bytes(n)).toBe(want);
	});

	it.each([
		[5, '5 s'],
		[125, '2 min'],
		[3 * 3600 + 12 * 60, '3 h 12 min'],
		[2 * 86400 + 3600, '2 d 1 h'],
	])('duration(%d) = %s', (s, want) => {
		expect(duration(s)).toBe(want);
	});

	it.each([
		['nvenc', 'NVIDIA (NVENC)'],
		['qsv', 'Intel (Quick Sync)'],
		['amf', 'AMD (AMF)'],
		['vaapi', 'VAAPI'],
		['videotoolbox', 'Apple (VideoToolbox)'],
		['cpu', 'CPU (sem aceleração)'],
		['', 'Indisponível (FFmpeg não encontrado)'],
		['new', 'new'],
	])('encoderName(%s) = %s', (e, want) => {
		expect(encoderName(e)).toBe(want);
	});
});
