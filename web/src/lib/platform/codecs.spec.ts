import { describe, expect, it } from 'vitest';

import { detectCapabilities } from './codecs';

const mp4AndWebm = (t: string) => t === 'video/mp4' || t === 'video/webm';
const someCodecs = (t: string) => /avc1\.64|mp4a\.40\.2|opus/.test(t);
const mp4AndHevc10 = (t: string) => t === 'video/mp4' || t.includes('hvc1.2');

describe('detectCapabilities', () => {
	it('asks containers to the element and codecs to MediaSource', () => {
		expect(detectCapabilities(mp4AndWebm, someCodecs)).toEqual([
			'mp4',
			'webm',
			'h264',
			'aac',
			'opus',
		]);
	});

	it('falls back to the element without MediaSource', () => {
		expect(detectCapabilities(mp4AndHevc10, undefined)).toEqual(['mp4', 'hevc:10']);
	});
});
