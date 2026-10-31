<script lang="ts">
	import type { Snippet } from 'svelte';

	import { imageSrcset } from '../api/images';
	import { focusGroup } from '../platform/nav.svelte';
	import { settings } from '../platform/settings.svelte';
	import { titleGradient } from './move';

	let {
		title,
		meta,
		imageId,
		color,
		trailerSrc,
		actions,
	}: {
		title: string;
		meta: string;
		imageId: string;
		color?: string;
		/** Muted trailer looping behind the hero (details pages). */
		trailerSrc?: string;
		actions: Snippet;
	} = $props();

	let playing = $state(false);
	const trailer = $derived(
		trailerSrc !== undefined && settings.backgroundTrailer && !settings.reduceMotion ? trailerSrc : '',
	);
</script>

<header class="hero" style:background={color ?? titleGradient(title)}>
	<img
		class="backdrop"
		src={`/api/images/${imageId}?variant=backdrop&w=1920`}
		srcset={imageSrcset(imageId, 'backdrop')}
		sizes="100vw"
		alt=""
		decoding="async"
		onerror={(e) => ((e.currentTarget as HTMLImageElement).style.display = 'none')}
	/>
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
	<div class="content">
		<h1>{title}</h1>
		{#if meta}<p class="meta">{meta}</p>{/if}
		<div class="actions" use:focusGroup={{ orientation: 'horizontal' }}>
			{@render actions()}
		</div>
	</div>
</header>

<style>
	.hero {
		position: relative;
		margin: 0 -5vw 2.5rem;
		height: 62vh;
		min-height: 22rem;
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
			linear-gradient(to top, #0b0b0f 0%, rgba(11, 11, 15, 0) 55%),
			linear-gradient(to right, rgba(11, 11, 15, 0.85) 0%, rgba(11, 11, 15, 0) 60%);
	}
	.content {
		position: absolute;
		left: 5vw;
		right: 40vw;
		bottom: 3rem;
	}
	h1 {
		margin: 0;
		font-size: 3.5rem;
		font-weight: 700;
		line-height: 1.05;
		letter-spacing: -0.02em;
	}
	.meta {
		margin: 1rem 0 0;
		font-size: 1.25rem;
		color: rgba(255, 255, 255, 0.6);
	}
	.actions {
		display: flex;
		margin-top: 1.5rem;
	}
	.actions > :global(* + *) {
		margin-left: 1rem;
	}
	/* TV: text of at least 24px at 1080p (AGENTS.md §7). */
	:global(html.tv) .meta {
		font-size: 1.5rem;
	}
</style>
