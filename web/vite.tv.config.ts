// Build of the TV apps' local page (web/src/shell → web/build-tv/index.html),
// copied into the .wgt/.ipk/.apk (docs/adr/008). Everything goes inline into
// one HTML file with a classic script: TV runtimes open it from file://,
// where module scripts, crossorigin stylesheets and font files may be blocked.

import { svelte } from '@sveltejs/vite-plugin-svelte';
import browserslist from 'browserslist';
import { browserslistToTargets } from 'lightningcss';
import { defineConfig, type Plugin } from 'vite';

import { jsTargets, legacyScopedCss, unoCSS } from './vite.config.ts';

const browsers = browserslist();

const escapeRegExp = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

/** Moves the JS and CSS files into index.html. */
export function inlineIntoHtml(
	html: string,
	scripts: Map<string, string>,
	styles: Map<string, string>,
): string {
	let page = html;
	for (const [file, code] of scripts) {
		page = page.replace(
			new RegExp(`<script[^>]*src="[^"]*${escapeRegExp(file)}"[^>]*></script>\\s*`),
			'',
		);
		// The single entry chunk has no imports or exports: wrapped in a
		// function it runs as a classic script, at the end of <body>.
		if (/^\s*(?:import|export)\b/m.test(code)) throw new Error(`${file}: not a standalone chunk`);
		const safe = code.replace(/<\/script/gi, '<\\/script');
		page = page.replace('</body>', () => `<script>(function(){\n${safe}\n})();</script>\n</body>`);
	}
	for (const [file, css] of styles) {
		page = page.replace(
			new RegExp(`<link[^>]*href="[^"]*${escapeRegExp(file)}"[^>]*>`),
			() => `<style>${css}</style>`,
		);
	}
	return page;
}

function singleFile(): Plugin {
	return {
		name: 'tv-single-file',
		apply: 'build',
		enforce: 'post',
		generateBundle(_options, bundle) {
			const html = Object.values(bundle).find((f) => f.fileName.endsWith('.html'));
			if (html === undefined || html.type !== 'asset') return;
			const scripts = new Map<string, string>();
			const styles = new Map<string, string>();
			for (const [name, file] of Object.entries(bundle)) {
				if (file.type === 'chunk') scripts.set(file.fileName, file.code);
				else if (file.fileName.endsWith('.css')) styles.set(file.fileName, String(file.source));
				else continue;
				delete bundle[name];
			}
			html.source = inlineIntoHtml(String(html.source), scripts, styles);
		},
	};
}

export default defineConfig({
	root: 'src/shell',
	base: './',
	publicDir: false,
	plugins: [
		svelte({ compilerOptions: { runes: true } }),
		unoCSS(),
		legacyScopedCss(),
		singleFile(),
	],
	css: {
		transformer: 'lightningcss',
		lightningcss: { targets: browserslistToTargets(browsers) },
	},
	build: {
		outDir: '../../build-tv',
		emptyOutDir: true,
		target: jsTargets(browsers),
		cssMinify: 'lightningcss',
		// Assets (the font) become data URLs: the page is a single file.
		assetsInlineLimit: () => true,
		modulePreload: false,
		// One chunk (inlined above); the CSS stays a separate asset so that
		// legacyScopedCss and scripts/check-build.ts see it.
		rolldownOptions: { output: { codeSplitting: false } },
	},
});
