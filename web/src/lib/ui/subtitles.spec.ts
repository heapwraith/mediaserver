import { describe, expect, it } from 'vitest';

import type { Subtitle } from '#lib/api/types.ts';

import { defaultSubtitle } from './subtitles';

const subs: Subtitle[] = [
	{ id: 'image', codec: 'hdmv_pgs_subtitle', kind: '', language: 'pt' },
	{ id: 'forced', codec: 'ass', kind: 'ass', language: 'pt', forced: true },
	{ id: 'regular', codec: 'subrip', kind: 'text', language: 'pt-BR' },
	{ id: 'english', codec: 'webvtt', kind: 'text', language: 'en' },
];

describe('defaultSubtitle', () => {
	it.each([
		['', 'en', -1],
		['pt-BR', 'ja', 2],
		['pt', '', 2],
		['pt-BR', 'pt-PT', 1],
		['en', 'en-US', -1],
		['en', 'ja', 3],
		['es', 'ja', -1],
	])('preference %s with audio %s selects %i', (pref, audio, expected) => {
		expect(defaultSubtitle(subs, pref, audio)).toBe(expected);
	});
	it('uses forced subtitles when no regular translation exists', () => {
		expect(defaultSubtitle(subs.slice(0, 2), 'pt', 'ja')).toBe(1);
	});
	it('keeps subtitles off with no supported match', () => {
		expect(defaultSubtitle(subs.slice(0, 1), 'pt', 'ja')).toBe(-1);
		expect(defaultSubtitle([], 'pt', 'ja')).toBe(-1);
	});
});
