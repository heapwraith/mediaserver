<script lang="ts" module>
	/** The app's icons: one stroke family drawn for Telão (no Unicode glyphs). */
	export type IconName =
		| 'play'
		| 'pause'
		| 'info'
		| 'check'
		| 'prev'
		| 'next'
		| 'back'
		| 'replay10'
		| 'forward10'
		| 'volume'
		| 'mute'
		| 'fullscreen'
		| 'plus'
		| 'restart';
</script>

<script lang="ts">
	let { name, size = '1.25em' }: { name: IconName; size?: string } = $props();
</script>

<svg
	class="icon"
	viewBox="0 0 24 24"
	width={size}
	height={size}
	aria-hidden="true"
	fill="none"
	stroke="currentColor"
	stroke-width="2.2"
	stroke-linecap="round"
	stroke-linejoin="round"
>
	{#if name === 'play'}
		<path d="M7 4.5v15a1 1 0 0 0 1.5.86l12.4-7.5a1 1 0 0 0 0-1.72L8.5 3.64A1 1 0 0 0 7 4.5Z" fill="currentColor" stroke="none" />
	{:else if name === 'pause'}
		<rect x="6" y="4.5" width="4" height="15" rx="1" fill="currentColor" stroke="none" />
		<rect x="14" y="4.5" width="4" height="15" rx="1" fill="currentColor" stroke="none" />
	{:else if name === 'info'}
		<circle cx="12" cy="12" r="9.5" />
		<path d="M12 11v6M12 7.2v.1" stroke-width="2.6" />
	{:else if name === 'check'}
		<path d="m5 12.5 4.5 4.5L19 7.5" stroke-width="2.6" />
	{:else if name === 'prev'}
		<path d="m15 5-7 7 7 7" stroke-width="2.6" />
	{:else if name === 'next'}
		<path d="m9 5 7 7-7 7" stroke-width="2.6" />
	{:else if name === 'back'}
		<path d="M19 12H6M11 6l-6 6 6 6" stroke-width="2.4" />
	{:else if name === 'replay10'}
		<path d="M4 12a8 8 0 1 0 2.6-5.9" />
		<path d="M4 3.5V8h4.5" />
		<text x="12" y="15.6" text-anchor="middle" font-size="7.5" font-weight="800" fill="currentColor" stroke="none">10</text>
	{:else if name === 'forward10'}
		<path d="M20 12a8 8 0 1 1-2.6-5.9" />
		<path d="M20 3.5V8h-4.5" />
		<text x="12" y="15.6" text-anchor="middle" font-size="7.5" font-weight="800" fill="currentColor" stroke="none">10</text>
	{:else if name === 'volume'}
		<path d="M4 9.5h3.5L12 5v14l-4.5-4.5H4Z" fill="currentColor" stroke="none" />
		<path d="M15.5 9a4 4 0 0 1 0 6M18 6.5a7.5 7.5 0 0 1 0 11" />
	{:else if name === 'mute'}
		<path d="M4 9.5h3.5L12 5v14l-4.5-4.5H4Z" fill="currentColor" stroke="none" />
		<path d="m16 9.5 5 5M21 9.5l-5 5" />
	{:else if name === 'fullscreen'}
		<path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5" />
	{:else if name === 'plus'}
		<path d="M12 5v14M5 12h14" stroke-width="2.6" />
	{:else if name === 'restart'}
		<path d="M4 12a8 8 0 1 0 2.6-5.9" />
		<path d="M4 3.5V8h4.5" />
	{/if}
</svg>

<style>
	.icon {
		flex: none;
		display: block;
	}
</style>
