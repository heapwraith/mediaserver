<script lang="ts" generics="T">
	import type { Snippet } from 'svelte';
	import { tick, untrack } from 'svelte';

	import { focusable } from '../platform/nav.svelte';
	import type { Action } from '../platform/keys';
	import { recall, remember } from './memory';
	import { moveIndex } from './move';

	// A grid (or a list, with one column) moved with the arrows as one nav
	// node. It renders every loaded item: long listings load page by page
	// through `onend`, so the DOM grows only as far as the user goes.
	let {
		id,
		items,
		key,
		cell,
		onopen,
		onend,
		minWidth = 18,
	}: {
		id: string;
		items: T[];
		key: (item: T) => string;
		cell: Snippet<[T, boolean]>;
		onopen: (item: T) => unknown;
		/** Called when the focus nears the end (load the next page). */
		onend?: () => unknown;
		/** Minimum column width in rem; 0 = a single column. */
		minWidth?: number;
	} = $props();

	let index = $state(untrack(() => recall(id).index));
	let grid: HTMLElement | undefined = $state();

	/** Columns as laid out by CSS: cells sharing the first cell's top. */
	function cols(): number {
		const cells = grid?.children;
		if (cells === undefined || cells.length === 0) return 1;
		const top = (cells[0] as HTMLElement).offsetTop;
		let n = 0;
		while (n < cells.length && (cells[n] as HTMLElement).offsetTop === top) n++;
		return Math.max(1, n);
	}

	async function select(i: number, scroll: boolean) {
		index = i;
		remember(id, index);
		if (onend !== undefined && index >= items.length - cols() * 2) void onend();
		if (!scroll) return;
		await tick();
		grid?.children[index]?.scrollIntoView({ block: 'nearest' });
	}

	function onKey(a: Action): boolean {
		const item = items[index];
		if (a === 'enter') {
			if (item !== undefined) void onopen(item);
			return item !== undefined;
		}
		if (a !== 'up' && a !== 'down' && a !== 'left' && a !== 'right') return false;
		const next = moveIndex(index, a, items.length, cols());
		if (next === null) return false;
		void select(next, true);
		return true;
	}
</script>

{#if items.length > 0}
	<div
		class="grid"
		style:grid-template-columns={minWidth > 0 ? `repeat(auto-fill, minmax(${minWidth}rem, 1fr))` : '1fr'}
		bind:this={grid}
		use:focusable={{ id, onKey }}
	>
		{#each items as item, i (key(item))}
			<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
			<div
				class="cell"
				onpointerenter={() => void select(i, false)}
				onclick={() => onopen(item)}
			>
				{@render cell(item, i === index)}
			</div>
		{/each}
	</div>
{/if}

<style>
	.grid {
		display: grid;
		grid-gap: 2rem 1.5rem;
		outline: none;
		/* room for the focused cell (scale 1.08) */
		padding: 1rem 0;
	}
	.cell {
		cursor: pointer;
		min-width: 0;
	}
</style>
