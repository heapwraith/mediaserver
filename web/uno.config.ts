import { presetLegacyCompat } from '@unocss/preset-legacy-compat';
import { defineConfig, presetWind3 } from 'unocss';

/**
 * Utilities that need more than Chromium 76 (AGENTS.md §6).
 * Use them only inside @supports in component CSS, never for essentials.
 */
const chromium76 = 'not supported on Chromium 76 (AGENTS.md §6)';

export default defineConfig({
	presets: [
		presetWind3(),
		presetLegacyCompat({ commaStyleColorFunction: true, legacyColorSpace: true }),
	],
	outputToCssLayers: false,
	blocklist: [
		// flex gap: Chrome 84. Use `grid-gap-*` (grid only) or `space-x-*` / `space-y-*`.
		[
			/^(?:[a-z-]+:)*gap(?:-[xy])?-/,
			{ message: `gap-*: ${chromium76}. Use grid-gap-* or space-*` },
		],
		[/^(?:[a-z-]+:)*aspect-/, { message: `aspect-ratio: ${chromium76}. Use padding or grid` }],
		[/^(?:[a-z-]+:)*-?inset-/, { message: `inset: ${chromium76}. Use top/right/bottom/left` }],
		[/^(?:[a-z-]+:)*(?:has|group-has|peer-has)-/, { message: `:has(): ${chromium76}` }],
		// `@sm:` and friends are container-query variants.
		[/^(?:[a-z-]+:)*@/, { message: `container queries: ${chromium76}` }],
		[/^(?:[a-z-]+:)*grid-(?:cols|rows)-subgrid$/, { message: `subgrid: ${chromium76}` }],
		// AGENTS.md §7: blur filters are too heavy for TVs. Use pre-rendered blurred images.
		[
			/^(?:[a-z-]+:)*(?:backdrop-|blur)/,
			{ message: 'blur/backdrop-filter is forbidden (AGENTS.md §7)' },
		],
	],
	rules: [
		// gap is safe on grid (Chrome 66+); this rule forces display:grid with it.
		[
			/^grid-gap-(\d+)$/,
			([, n]) => ({ display: 'grid', gap: `${Number(n) * 0.25}rem` }),
			{ autocomplete: 'grid-gap-<num>' },
		],
	],
	theme: {
		// Telão (docs/adr/013-identidade-telao.md). The same values live as CSS
		// variables in src/app.css for the components' own <style>.
		colors: {
			bg: '#000000',
			surface: {
				1: '#121212',
				2: '#1C1C1E',
				3: '#2A2A2D',
			},
			text: {
				primary: '#F5F5F4',
				secondary: '#A8A29E',
				tertiary: '#8F8882',
			},
			accent: '#FF4D2E',
		},
		fontFamily: {
			sans: '"Archivo Variable", Archivo, system-ui, -apple-system, "Segoe UI", Roboto, sans-serif',
			display: '"Bebas Neue", "Archivo Variable", Impact, sans-serif',
		},
		borderRadius: {
			card: '10px',
			pill: '999px',
		},
	},
});
