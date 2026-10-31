<script lang="ts">
	import type { Snippet } from 'svelte';

	import type { Action } from '../platform/keys';
	import { focusable } from '../platform/nav.svelte';

	let {
		onclick,
		primary = false,
		selected = false,
		id,
		onKey,
		children,
	}: {
		onclick: () => unknown;
		primary?: boolean;
		selected?: boolean;
		id?: string;
		/** Handles a key while focused; returns true when it consumed it. */
		onKey?: (a: Action) => boolean;
		children: Snippet;
	} = $props();
</script>

<button class="btn" class:primary class:selected use:focusable={{ id, onKey }} onclick={() => void onclick()}>
	{@render children()}
</button>

<style>
	.btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-height: 3rem;
		padding: 0 1.75rem;
		border: 0;
		border-radius: 999px;
		background: rgba(255, 255, 255, 0.14);
		color: rgba(255, 255, 255, 0.92);
		font: inherit;
		font-size: 1.125rem;
		font-weight: 600;
		white-space: nowrap;
		cursor: pointer;
		outline: none;
		transition: transform 200ms ease-out;
	}
	.primary,
	.selected {
		background: rgba(255, 255, 255, 0.92);
		color: #0b0b0f;
	}
	.btn:global([data-focused]) {
		transform: scale(1.08);
		box-shadow:
			0 12px 30px rgba(0, 0, 0, 0.5),
			0 0 0 3px #4c8dff;
	}
	/* TV: text of at least 24px at 1080p (AGENTS.md §7). */
	:global(html.tv) .btn {
		min-height: 4rem;
		padding: 0 2.25rem;
		font-size: 1.5rem;
	}
</style>
