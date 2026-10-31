import { describe, expect, it } from 'vitest';

import { checkCss, inlineStyles, staticImports, withoutSupports } from './scripts/check-build';
import { unwrapSvelteWhere } from './vite.config';
import { inlineIntoHtml } from './vite.tv.config';

describe('checkCss', () => {
	it('accepts Chromium 76 CSS', () => {
		expect(
			checkCss(
				'.a{display:grid;gap:1rem}.b{grid-gap:2rem}.c{box-shadow:inset 0 0 1px red}.d{transition-property:opacity,backdrop-filter}',
			),
		).toEqual([]);
	});

	it.each([
		['.a{aspect-ratio:16/9}', 'aspect-ratio'],
		['.a{inset:0}', 'inset'],
		[':where(.a){color:red}', ':where()'],
		['.a:has(img){color:red}', ':has()'],
		['@layer base{.a{color:red}}', '@layer'],
		['.a{backdrop-filter:blur(4px)}', 'backdrop-filter'],
		['.a{filter:blur(4px)}', 'blur()'],
		['.a{display:flex;gap:1rem}', 'gap outside'],
	])('rejects %s', (css, want) => {
		expect(checkCss(css).join()).toContain(want);
	});

	it('allows anything inside @supports', () => {
		expect(withoutSupports('.a{x:1}@supports (inset:0){.b{inset:0}}.c{y:2}')).toBe(
			'.a{x:1}.c{y:2}',
		);
		expect(checkCss('@supports (aspect-ratio:1){.a{aspect-ratio:1}}')).toEqual([]);
	});
});

describe('unwrapSvelteWhere', () => {
	it('keeps the scoping class without :where()', () => {
		expect(unwrapSvelteWhere('.a.svelte-1x b:where(.svelte-1x){color:red}')).toBe(
			'.a.svelte-1x b.svelte-1x{color:red}',
		);
		expect(checkCss(unwrapSvelteWhere('p:where(.svelte-ab9){x:1}'))).toEqual([]);
	});
});

describe('staticImports', () => {
	it('skips dynamic imports', () => {
		expect(staticImports('import{a}from"./a.js";import"./b.js";import("./c.js")')).toEqual([
			'./a.js',
			'./b.js',
		]);
	});
});

describe('TV page', () => {
	const html =
		'<html><head><script type="module" crossorigin src="./assets/index-a1.js"></script>\n' +
		'<link rel="stylesheet" crossorigin href="./assets/index-b2.css"></head><body></body></html>';

	it('inlines the script (as a classic one, at the end) and the CSS', () => {
		const page = inlineIntoHtml(
			html,
			new Map([['assets/index-a1.js', 'const s = "</script>";console.log(s)']]),
			new Map([['assets/index-b2.css', '.a{color:red}']]),
		);
		expect(page).not.toContain('src=');
		expect(page).not.toContain('href=');
		expect(page).toContain('<style>.a{color:red}</style>');
		expect(page).toContain(
			'<body><script>(function(){\nconst s = "<\\/script>";console.log(s)\n})();</script>\n</body>',
		);
		expect(inlineStyles(page)).toEqual(['.a{color:red}']);
	});

	it('refuses a chunk that imports others', () => {
		expect(() =>
			inlineIntoHtml(html, new Map([['assets/index-a1.js', 'import "./x.js";']]), new Map()),
		).toThrow('not a standalone chunk');
	});
});
