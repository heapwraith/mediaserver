<script lang="ts">
	import type { Snippet } from 'svelte';

	import { imageSrcset } from '../api/images';
	import { focusGroup } from '../platform/nav.svelte';
	import { settings } from '../platform/settings.svelte';
	import { titleGradient } from './move';

	let {
		title,
		meta,
		badges = [],
		status,
		imageId,
		color,
		trailerSrc,
		compact = false,
		actions,
	}: {
		title: string;
		meta: string;
		badges?: string[];
		/** Short status in the accent color, before the meta line. */
		status?: string;
		imageId: string;
		color?: string;
		/** Muted trailer looping behind the hero (details pages). */
		trailerSrc?: string;
		/** Shorter hero for details pages (the episodes start sooner). */
		compact?: boolean;
		actions: Snippet;
	} = $props();

	let playing = $state(false);
	let failed = $state(false);
	const trailer = $derived(
		trailerSrc !== undefined && settings.backgroundTrailer && !settings.reduceMotion ? trailerSrc : '',
	);
</script>

<!-- Telão (ADR 013): the scene owns the right of the screen and fades into the black. -->
<header class="hero" class:compact>
	<div class="scene" style:background={failed ? (color ?? titleGradient(title)) : undefined}>
		{#if !failed}
			<img
				class="backdrop"
				src={`/api/images/${imageId}?variant=backdrop&w=1920`}
				srcset={imageSrcset(imageId, 'backdrop')}
				sizes="73vw"
				alt=""
				decoding="async"
				onerror={() => (failed = true)}
			/>
		{/if}
		{#if trailer !== ''}
			<!-- Decorative: muted, no controls (AGENTS.md §7). -->
			<video
				class="backdrop trailer"
				class:playing
				src={trailer}
				muted
				loop
				autoplay
				playsinline
				preload="auto"
				onplaying={() => (playing = true)}
			></video>
		{/if}
		<div class="shade"></div>
	</div>
	<div class="content">
		<h1 class:long={title.length > 14}>{title}</h1>
		<p class="meta">
			{#if status}<span class="status">{status}</span>{/if}
			{#if meta}<span>{meta}</span>{/if}
			{#each badges as b (b)}<span class="badge">{b}</span>{/each}
		</p>
		<div class="actions" use:focusGroup={{ orientation: 'horizontal' }}>
			{@render actions()}
		</div>
	</div>
</header>

<style>
	.hero {
		position: relative;
		/* full bleed: under the rail on the left, to the screen edge on the right */
		margin: 0 -5vw 0 calc(-1 * var(--content-left));
		height: 74vh;
		min-height: 26rem;
	}
	.hero.compact {
		height: 60vh;
	}
	.scene {
		position: absolute;
		top: 0;
		right: 0;
		width: 73%;
		height: 100%;
		overflow: hidden;
	}
	.backdrop,
	.shade {
		position: absolute;
		top: 0;
		left: 0;
		width: 100%;
		height: 100%;
	}
	.backdrop {
		object-fit: cover;
	}
	.trailer {
		opacity: 0;
		transition: opacity 400ms ease-out;
	}
	.trailer.playing {
		opacity: 1;
	}
	.shade {
		background:
			linear-gradient(90deg, #000 0%, rgba(0, 0, 0, 0.7) 26%, rgba(0, 0, 0, 0) 55%),
			linear-gradient(0deg, #000 0%, rgba(0, 0, 0, 0) 38%);
	}
	.content {
		position: absolute;
		left: var(--content-left);
		top: 18%;
		width: 57.5rem;
		max-width: 60vw;
	}
	.compact .content {
		top: 16%;
	}
	h1 {
		margin: 0 0 1.375rem;
		font-family: var(--font-display);
		font-size: 10.5rem;
		font-weight: 400;
		line-height: 0.86;
		letter-spacing: 0.005em;
		text-transform: uppercase;
	}
	.compact h1 {
		font-size: 8.5rem;
	}
	.meta {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		margin: 0 0 1.75rem;
		font-size: 1.625rem;
		font-weight: 700;
		color: var(--text-2);
	}
	.meta > span {
		margin-right: 1.25rem;
	}
	.status {
		color: var(--accent);
	}
	.badge {
		padding: 0 0.5rem;
		border: 2px solid var(--text-3);
		border-radius: 4px;
		color: var(--text);
		font-size: 1.25rem;
	}
	.actions {
		display: flex;
	}
	.actions > :global(* + *) {
		margin-left: 1rem;
	}
	/* Long titles: smaller, so they stay within two lines. */
	h1.long {
		font-size: 7.25rem;
	}
	.compact h1.long {
		font-size: 6.5rem;
	}
</style>
