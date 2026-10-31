import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import browserslist from 'browserslist';
import { browserslistToTargets } from 'lightningcss';
import UnoCSS from 'unocss/vite';
import type { Plugin } from 'vite';
import { defineConfig } from 'vitest/config';

// Targets come from web/.browserslistrc (the single source, AGENTS.md §6),
// found from the current folder: every npm script runs inside web/.
const browsers = browserslist();

/** Lowest version per browser family, in the "chrome76" format used by Vite. */
export function jsTargets(list: string[]): string[] {
	const names: Record<string, string> = {
		chrome: 'chrome',
		edge: 'edge',
		firefox: 'firefox',
		safari: 'safari',
		ios_saf: 'ios',
	};
	const min = new Map<string, number>();
	for (const entry of list) {
		const [family = '', version = ''] = entry.split(' ');
		const name = names[family];
		const v = Number.parseFloat(version.split('-')[0] ?? '');
		if (name === undefined || Number.isNaN(v)) continue;
		const current = min.get(name);
		if (current === undefined || v < current) min.set(name, v);
	}
	return [...min].map(([name, v]) => `${name}${v}`);
}

/**
 * UnoCSS without its `transformIndexHtml` hooks. They only scan/transform a
 * Vite index.html, which SvelteKit does not use (and warns about); the CSS
 * comes from `virtual:uno.css` instead.
 */
export function unoCSS(): Plugin[] {
	return UnoCSS().map(({ transformIndexHtml: _unused, ...plugin }) => plugin);
}

/**
 * Svelte 5 scopes some selectors with `:where(.svelte-xyz)`, which Chromium
 * 76 (Chrome 88+ only) rejects, dropping the whole rule on 2021 TVs. In the
 * build, the plain class does the same job (one class more of specificity).
 */
export function unwrapSvelteWhere(css: string): string {
	return css.replace(/:where\((\.svelte-[\w-]+)\)/g, '$1');
}

export function legacyScopedCss(): Plugin {
	return {
		name: 'legacy-scoped-css',
		apply: 'build',
		enforce: 'post',
		generateBundle(_options, bundle) {
			for (const file of Object.values(bundle)) {
				if (
					file.type === 'asset' &&
					file.fileName.endsWith('.css') &&
					typeof file.source === 'string'
				) {
					file.source = unwrapSvelteWhere(file.source);
				}
			}
		},
	};
}

export default defineConfig({
	optimizeDeps: { exclude: ['jassub'] },
	plugins: [
		legacyScopedCss(),
		unoCSS(),
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true,
			},
			// SPA: every unknown path falls back to index.html (served by the Go binary).
			adapter: adapter({ fallback: 'index.html' }),
		}),
	],
	css: {
		transformer: 'lightningcss',
		lightningcss: { targets: browserslistToTargets(browsers) },
	},
	build: {
		target: jsTargets(browsers),
		cssMinify: 'lightningcss',
	},
	server: {
		proxy: { '/api': 'http://localhost:8080' },
	},
	test: {
		expect: { requireAssertions: true },
		include: ['src/**/*.{test,spec}.{js,ts}', '*.spec.ts'],
		environment: 'node',
	},
});
