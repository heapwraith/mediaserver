<script lang="ts">
	import type { Snippet } from 'svelte';

	import type { Action } from '../platform/keys';
	import { focusable } from '../platform/nav.svelte';
	import Icon, { type IconName } from './Icon.svelte';

	let {
		onclick,
		primary = false,
		selected = false,
		id,
		onKey,
		icon,
		children,
	}: {
		onclick: () => unknown;
		primary?: boolean;
		selected?: boolean;
		id?: string;
		/** Handles a key while focused; returns true when it consumed it. */
		onKey?: (a: Action) => boolean;
		/** Drawn before the label (no Unicode glyphs in labels). */
		icon?: IconName;
		children: Snippet;
	} = $props();
</script>

<button class="btn" class:primary class:selected use:focusable={{ id, onKey }} onclick={() => void onclick()}>
	{#if icon}<Icon name={icon} />{/if}
	{@render children()}
</button>

<style>
	.btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-height: 3.5rem;
		padding: 0 2rem;
		border: 0;
		border-radius: var(--radius);
		background: rgba(120, 113, 108, 0.45);
		color: var(--text);
		font: inherit;
		font-size: 1.25rem;
		font-weight: 800;
		white-space: nowrap;
		cursor: pointer;
		outline: none;
		transition:
			transform 200ms cubic-bezier(0.2, 0.8, 0.2, 1),
			box-shadow 200ms ease-out;
	}
	.btn > :global(.icon) {
		margin-right: 0.75rem;
	}
	.primary,
	.selected {
		background: var(--text);
		color: #000;
	}
	.btn:global([data-focused]) {
		transform: scale(1.06);
		box-shadow:
			0 0 0 4px var(--ring),
			0 16px 32px rgba(0, 0, 0, 0.5);
	}
	/* TV: text of at least 24px at 1080p (AGENTS.md §7). */
	:global(html.tv) .btn {
		min-height: 4.875rem;
		padding: 0 2.5rem;
		font-size: 1.75rem;
	}
</style>
