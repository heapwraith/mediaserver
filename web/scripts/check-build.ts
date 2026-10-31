// Checks the production build (run by `make check` after `vite build`):
//   1. the JS needed to open the home page is < 150 KB gzip (AGENTS.md §8);
//   2. the final CSS uses nothing Chromium 76 lacks (AGENTS.md §6) and no
//      blur/backdrop-filter (§7). Rules inside @supports are allowed.
// The TV page (web/build-tv, CSS inlined in its index.html) gets check 2.
// Usage: node scripts/check-build.ts [build dir] [TV page dir]

import { readFileSync, readdirSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { gzipSync } from 'node:zlib';

export const BUDGET_GZIP = 150 * 1024;

const forbidden: [RegExp, string][] = [
	[/aspect-ratio/, 'aspect-ratio (Chrome 88)'],
	[/(?:^|[;{])inset:/, 'inset (Chrome 87)'],
	[/:where\(/, ':where() (Chrome 88)'],
	[/:is\(/, ':is() (Chrome 88)'],
	[/:has\(/, ':has() (Chrome 105)'],
	[/@layer/, '@layer (Chrome 99)'],
	[/@container/, 'container queries (Chrome 105)'],
	[/subgrid/, 'subgrid (Chrome 117)'],
	// As a property; the name alone may sit in a transition-property list.
	[/(?:^|[;{])\s*(?:-webkit-)?backdrop-filter\s*:/, 'backdrop-filter (AGENTS.md §7)'],
	[/blur\(/, 'blur() (AGENTS.md §7)'],
];

/** Removes @supports blocks: what is inside them is an optional enhancement. */
export function withoutSupports(css: string): string {
	let out = '';
	let i = 0;
	while (i < css.length) {
		const at = css.indexOf('@supports', i);
		if (at < 0) return out + css.slice(i);
		out += css.slice(i, at);
		let depth = 0;
		let j = css.indexOf('{', at);
		if (j < 0) return out;
		for (; j < css.length; j++) {
			if (css[j] === '{') depth++;
			else if (css[j] === '}' && --depth === 0) break;
		}
		i = j + 1;
	}
	return out;
}

/** Problems of one stylesheet (an empty list when it is fine). */
export function checkCss(css: string): string[] {
	const plain = withoutSupports(css);
	const problems = forbidden.filter(([re]) => re.test(plain)).map(([, name]) => name);
	// gap only on grids: flex gap needs Chrome 84. Approximation: the rule
	// declaring gap must also declare display:grid (grid-gap is always fine).
	for (const m of plain.matchAll(/\{([^{}]*)\}/g)) {
		const decls = m[1] ?? '';
		if (
			/(?:^|;)\s*(?:row-|column-)?gap\s*:/.test(decls) &&
			!/display\s*:\s*(?:inline-)?grid/.test(decls)
		) {
			problems.push(`gap outside a display:grid rule: {${decls.slice(0, 80)}}`);
		}
	}
	return problems;
}

/** The CSS of the <style> blocks of a page. */
export function inlineStyles(html: string): string[] {
	return [...html.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1] ?? '');
}

/** Static imports (not dynamic import()) of a built JS file. */
export function staticImports(js: string): string[] {
	return [...js.matchAll(/(?:\bfrom|\bimport)\s*["']([^"']+\.js)["']/g)].map((m) => m[1] ?? '');
}

function initialJs(build: string): string[] {
	const html = readFileSync(join(build, 'index.html'), 'utf8');
	const start = [...html.matchAll(/href="\/([^"]+\.js)"\s+rel="modulepreload"/g)].map(
		(m) => m[1] ?? '',
	);
	// The home route: its node number is in the router dictionary of app.js.
	const app = start.find((f) => /entry\/app\./.test(f));
	const home =
		app === undefined
			? undefined
			: /"\/":\[~?(\d+)/.exec(readFileSync(join(build, app), 'utf8'))?.[1];
	const nodes = join(build, '_app/immutable/nodes');
	if (home !== undefined) {
		const file = readdirSync(nodes).find((f) => f.startsWith(`${home}.`) && f.endsWith('.js'));
		if (file !== undefined) start.push(`_app/immutable/nodes/${file}`);
	}
	const seen = new Set<string>();
	const queue = start.map((f) => resolve(build, f));
	while (queue.length > 0) {
		const file = queue.pop() ?? '';
		if (seen.has(file)) continue;
		seen.add(file);
		for (const dep of staticImports(readFileSync(file, 'utf8'))) {
			queue.push(dep.startsWith('/') ? resolve(build, dep.slice(1)) : resolve(dirname(file), dep));
		}
	}
	return [...seen];
}

const kb = (n: number) => `${(n / 1024).toFixed(1)} KB`;

function main(build: string, tv?: string): number {
	let failed = false;
	const js = initialJs(build);
	const size = js.reduce((n, f) => n + gzipSync(readFileSync(f)).length, 0);
	console.log(`initial JS: ${js.length} files, ${kb(size)} gzip (budget ${kb(BUDGET_GZIP)})`);
	if (size > BUDGET_GZIP) failed = true;

	const assets = join(build, '_app/immutable/assets');
	for (const f of readdirSync(assets).filter((n) => n.endsWith('.css'))) {
		for (const p of checkCss(readFileSync(join(assets, f), 'utf8'))) {
			console.error(`${f}: ${p}`);
			failed = true;
		}
	}
	if (tv !== undefined) {
		const html = readFileSync(join(tv, 'index.html'), 'utf8');
		console.log(`TV page: ${kb(gzipSync(html).length)} gzip`);
		for (const p of inlineStyles(html).flatMap(checkCss)) {
			console.error(`${tv}/index.html: ${p}`);
			failed = true;
		}
	}
	console.log(failed ? 'check-build: FAILED' : 'check-build: ok');
	return failed ? 1 : 0;
}

if (import.meta.main) process.exit(main(process.argv[2] ?? 'build', process.argv[3]));
