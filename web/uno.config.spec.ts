import { createGenerator } from 'unocss';
import { describe, expect, it } from 'vitest';

import config from './uno.config';

async function css(classes: string): Promise<string> {
	const uno = await createGenerator(config);
	const result = await uno.generate(classes, { preflights: false });
	return result.css;
}

describe('uno.config blocklist (AGENTS.md §6 and §7)', () => {
	it.each([
		['flex gap', 'gap-4'],
		['flex gap x', 'gap-x-2'],
		['aspect-ratio', 'aspect-video'],
		['inset', 'inset-0'],
		['negative inset', '-inset-x-1'],
		[':has()', 'has-[img]:p-2'],
		['container query', '@sm:p-2'],
		['container', '@container'],
		['subgrid', 'grid-cols-subgrid'],
		['blur filter', 'blur-sm'],
		['backdrop-filter', 'backdrop-blur-md'],
		['variant + blocked', 'hover:gap-2'],
	])('blocks %s (%s)', async (_label, cls) => {
		expect(await css(cls)).toBe('');
	});

	it('keeps the allowed alternatives', async () => {
		const out = await css('grid-gap-4 space-x-2 top-0 left-0 bg-bg text-text-primary rounded-card');
		expect(out).toContain('display:grid');
		expect(out).toContain('gap:1rem');
		expect(out).toContain('margin-left');
		expect(out).toContain('top:0');
		expect(out).toContain('background-color');
	});

	it('emits CSS that Chromium 76 understands', async () => {
		const out = await css('bg-bg text-text-primary bg-accent/50 space-y-4');
		expect(out).not.toContain('@layer');
		expect(out).not.toContain(':where(');
		// legacy-compat: comma-style rgb()/rgba(), no space-separated colors
		expect(out).not.toMatch(/rgb\(\d+ \d+ \d+/);
	});
});
