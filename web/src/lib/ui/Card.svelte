<script lang="ts">
	import { imageSrcset } from '../api/images';
	import type { CardItem } from './cards';
	import Icon from './Icon.svelte';
	import { titleGradient } from './move';

	let {
		item,
		current = false,
		sizes = '(min-width: 2560px) 960px, 480px',
	}: { item: CardItem; current?: boolean; sizes?: string } = $props();

	let failed = $state(false);
	let loaded = $state(false);
</script>

<!-- Telão (ADR 013): the title sits on the picture, never repeated below it. -->
<div class="card" class:current class:bare={failed}>
	<div class="frame" style:background={item.color ?? titleGradient(item.title)}>
		{#if !failed}
			<img
				src={`/api/images/${item.imageId}?variant=card&w=480`}
				srcset={imageSrcset(item.imageId)}
				{sizes}
				alt=""
				loading="lazy"
				decoding="async"
				class:loaded
				onload={() => (loaded = true)}
				onerror={() => (failed = true)}
			/>
		{/if}
		<div class="shade"></div>
		<div class="label" class:with-bar={item.progress !== undefined}>
			<p class="title">{item.title}</p>
			{#if item.subtitle}<p class="subtitle">{item.subtitle}</p>{/if}
		</div>
		{#if item.progress !== undefined}
			<div class="bar"><div class="fill" style:transform={`scaleX(${item.progress})`}></div></div>
		{/if}
		{#if item.watched}<span class="check" role="img" aria-label="Assistido"><Icon name="check" size="1.25rem" /></span>{/if}
	</div>
</div>

<style>
	.frame {
		position: relative;
		padding-top: 56.25%; /* 16:9 without aspect-ratio (Chromium 76) */
		border-radius: var(--radius);
		overflow: hidden;
		background: var(--panel);
		transition:
			transform 220ms cubic-bezier(0.2, 0.8, 0.2, 1),
			box-shadow 220ms ease-out;
	}
	img,
	.shade {
		position: absolute;
		top: 0;
		left: 0;
		width: 100%;
		height: 100%;
	}
	img {
		object-fit: cover;
		opacity: 0;
		transition: opacity 200ms ease-out;
	}
	img.loaded {
		opacity: 1;
	}
	.shade {
		background: linear-gradient(0deg, rgba(0, 0, 0, 0.85) 0%, rgba(0, 0, 0, 0) 58%);
	}
	.label {
		position: absolute;
		left: 1.125rem;
		right: 1.125rem;
		bottom: 1rem;
	}
	.label.with-bar {
		bottom: 1.375rem;
	}
	.title,
	.subtitle {
		margin: 0;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.title {
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		font-family: var(--font-display);
		font-size: 2.5rem;
		font-weight: 400;
		line-height: 0.95;
		letter-spacing: 0.01em;
		text-transform: uppercase;
		color: var(--text);
	}
	.subtitle {
		margin-top: 0.25rem;
		white-space: nowrap;
		font-size: 1.125rem;
		font-weight: 600;
		color: #d6d3d1;
	}
	/* No picture: the title alone, large, over the dominant-color gradient. */
	.bare .title {
		font-size: 3.25rem;
		-webkit-line-clamp: 3;
		line-clamp: 3;
	}
	.bar {
		position: absolute;
		left: 0;
		right: 0;
		bottom: 0;
		height: 0.375rem;
		background: rgba(255, 255, 255, 0.25);
		overflow: hidden;
	}
	.fill {
		width: 100%;
		height: 100%;
		background: var(--accent);
		transform-origin: left center;
	}
	.check {
		position: absolute;
		top: 0.75rem;
		right: 0.75rem;
		display: flex;
		align-items: center;
		justify-content: center;
		width: 2.25rem;
		height: 2.25rem;
		border-radius: 999px;
		background: rgba(0, 0, 0, 0.75);
		color: var(--text);
	}
	/* Focus (AGENTS.md §7): the row/grid holding the card has data-focused. */
	:global([data-focused]) .current .frame {
		transform: scale(1.1);
		box-shadow: var(--focus-ring);
		z-index: 2;
	}
	/* TV: text of at least 24px at 1080p (AGENTS.md §7). */
	:global(html.tv) .subtitle {
		font-size: 1.5rem;
	}
</style>
