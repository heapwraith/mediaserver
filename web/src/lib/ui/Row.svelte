<script lang="ts">
	import { untrack } from 'svelte';

	import { goto } from '$app/navigation';

	import { focusable } from '../platform/nav.svelte';
	import type { Action } from '../platform/keys';
	import Card from './Card.svelte';
	import Icon from './Icon.svelte';
	import type { CardItem } from './cards';
	import { recall, remember } from './memory';
	import { moveIndex, rowStart, rowWindow } from './move';

	let {
		id,
		title,
		items,
		onopen = (item: CardItem) => goto(item.href),
	}: {
		id: string;
		title: string;
		items: CardItem[];
		onopen?: (item: CardItem) => unknown;
	} = $props();

	// Card width and spacing, in rem (the root font size scales with the screen).
	const CARD = 24;
	const SPACE = 1.125;
	const PITCH = CARD + SPACE;

	const saved = untrack(() => recall(id));
	let index = $state(saved.index);
	let start = $state(saved.start);
	let width = $state(0);

	const remPx = () => parseFloat(getComputedStyle(document.documentElement).fontSize) || 16;
	const visible = $derived(Math.max(1, Math.floor((width + SPACE * remPx()) / (PITCH * remPx()))));
	const range = $derived(rowWindow(start, visible, items.length));
	const shown = $derived(items.slice(range[0], range[1]));

	function select(i: number, scroll: boolean) {
		index = Math.min(Math.max(0, i), items.length - 1);
		if (scroll) start = rowStart(start, index, visible, items.length);
		remember(id, index, start);
	}

	function onKey(a: Action): boolean {
		if (a === 'enter') {
			const item = items[index];
			if (item !== undefined) void onopen(item);
			return item !== undefined;
		}
		if (a !== 'left' && a !== 'right') return false;
		const next = moveIndex(index, a, items.length, items.length);
		if (next === null) return false;
		select(next, true);
		return true;
	}

	function page(dir: -1 | 1) {
		start = Math.max(0, Math.min(start + dir * visible, items.length - visible));
		remember(id, index, start);
	}
</script>

{#if items.length > 0}
	<section class="row" use:focusable={{ id, onKey }}>
		<h2>{title}</h2>
		<div class="viewport" bind:clientWidth={width}>
			<div class="track" style:transform={`translateX(-${start * PITCH}rem)`}>
				<div style:width={`${range[0] * PITCH}rem`} class="spacer"></div>
				{#each shown as item, i (item.key)}
					<!-- Keys reach the row through the nav layer; the mouse uses these handlers. -->
					<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
					<div
						class="slot"
						style:width={`${CARD}rem`}
						style:margin-right={`${SPACE}rem`}
						onpointerenter={() => select(range[0] + i, false)}
						onclick={() => onopen(item)}
					>
						<Card {item} current={range[0] + i === index} />
					</div>
				{/each}
			</div>
			{#if start > 0}
				<button class="page prev" tabindex="-1" aria-label="Anteriores" onclick={() => page(-1)}
					><Icon name="prev" size="2.5rem" /></button
				>
			{/if}
			{#if start + visible < items.length}
				<button class="page next" tabindex="-1" aria-label="Próximos" onclick={() => page(1)}
					><Icon name="next" size="2.5rem" /></button
				>
			{/if}
		</div>
	</section>
{/if}

<style>
	.row {
		outline: none;
		margin-bottom: 2.5rem;
	}
	h2 {
		margin: 0 0 0.5rem;
		font-size: 1.875rem;
		font-weight: 800;
		color: var(--text);
	}
	.viewport {
		position: relative;
		/* room for the focused card (scale 1.1) and its shadow */
		padding: 1.25rem 1.5rem 1.75rem;
		margin: -0.5rem -1.5rem -1.25rem;
		overflow: hidden;
	}
	.track {
		display: flex;
		transition: transform 250ms ease-out;
		will-change: transform;
	}
	.spacer,
	.slot {
		flex: none;
		cursor: pointer;
	}
	.page {
		position: absolute;
		top: 0;
		bottom: 1.75rem;
		width: 3.5rem;
		display: flex;
		align-items: center;
		justify-content: center;
		border: 0;
		background: rgba(0, 0, 0, 0.75);
		color: var(--text);
		cursor: pointer;
		opacity: 0;
		transition: opacity 150ms ease-out;
	}
	.viewport:hover .page {
		opacity: 1;
	}
	.prev {
		left: 0;
	}
	.next {
		right: 0;
	}
</style>
