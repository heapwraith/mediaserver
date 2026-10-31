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
		colors: {
			bg: '#0B0B0F',
			surface: {
				1: '#15151B',
				2: '#1E1E26',
				3: '#282832',
			},
			text: {
				primary: 'rgba(255, 255, 255, 0.92)',
				secondary: 'rgba(255, 255, 255, 0.60)',
				tertiary: 'rgba(255, 255, 255, 0.40)',
			},
			accent: '#4C8DFF',
		},
		fontFamily: {
			sans: '"Inter Variable", Inter, system-ui, -apple-system, "Segoe UI", Roboto, sans-serif',
		},
		borderRadius: {
			card: '12px',
			pill: '999px',
		},
	},
});
