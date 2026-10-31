<script lang="ts">
	import { imageSrcset } from '../api/images';
	import type { CardItem } from './cards';
	import { titleGradient } from './move';

	let {
		item,
		current = false,
		sizes = '(min-width: 2560px) 960px, 480px',
	}: { item: CardItem; current?: boolean; sizes?: string } = $props();

	let failed = $state(false);
	let loaded = $state(false);
</script>

<div class="card" class:current>
	<div class="frame" style:background={item.color ?? titleGradient(item.title)}>
		{#if failed}
			<span class="type-title">{item.title}</span>
		{:else}
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
	</div>
	<p class="title">{item.title}</p>
	{#if item.subtitle}<p class="subtitle">{item.subtitle}</p>{/if}
</div>

<style>
	.frame {
		position: relative;
		padding-top: 56.25%; /* 16:9 without aspect-ratio (Chromium 76) */
		border-radius: 12px;
		overflow: hidden;
		transition: transform 200ms ease-out;
	}
	img,
	.type-title {
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
	.type-title {
		display: flex;
		align-items: flex-end;
		box-sizing: border-box;
		padding: 1rem;
		font-size: 1.5rem;
		font-weight: 700;
		line-height: 1.1;
		color: rgba(255, 255, 255, 0.92);
	}
	.title,
	.subtitle {
		margin: 0.75rem 0 0;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		font-size: 1rem;
		color: rgba(255, 255, 255, 0.92);
	}
	.subtitle {
		margin-top: 0.25rem;
		font-size: 0.875rem;
		color: rgba(255, 255, 255, 0.6);
	}
	/* Focus (AGENTS.md §7): the row/grid holding the card has data-focused. */
	:global([data-focused]) .current .frame {
		transform: scale(1.08);
		box-shadow:
			0 16px 40px rgba(0, 0, 0, 0.6),
			0 0 0 3px rgba(255, 255, 255, 0.92);
	}
</style>
