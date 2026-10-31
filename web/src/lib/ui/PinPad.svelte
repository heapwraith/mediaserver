<script lang="ts">
	import { onMount } from 'svelte';

	import { digitKey, isDirection, type Action } from '../platform/keys';
	import { focusId, focusable } from '../platform/nav.svelte';
	import { moveIndex } from './move';

	// 4-digit PIN entry for the remote: one nav node over a 3×4 keypad
	// (arrows + Enter), plus the number keys of the keyboard or remote.
	// Voltar erases a digit, then closes the pad.

	let {
		id = 'pin',
		title,
		error = '',
		onsubmit,
		oncancel,
	}: {
		id?: string;
		title: string;
		error?: string;
		/** Called with the 4 digits; resolve false to clear them (wrong PIN). */
		onsubmit: (pin: string) => Promise<boolean>;
		oncancel: () => unknown;
	} = $props();

	const LENGTH = 4;
	const keys = ['1', '2', '3', '4', '5', '6', '7', '8', '9', 'cancel', '0', 'erase'] as const;
	type Key = (typeof keys)[number];

	let pin = $state('');
	let index = $state(0);
	let busy = $state(false);

	onMount(() => {
		// After the nav tree picked up this node.
		requestAnimationFrame(() => focusId(id));
	});

	async function press(k: Key) {
		if (busy) return;
		if (k === 'cancel') {
			void oncancel();
			return;
		}
		if (k === 'erase') {
			pin = pin.slice(0, -1);
			return;
		}
		if (pin.length >= LENGTH) return;
		pin += k;
		if (pin.length === LENGTH) {
			busy = true;
			const ok = await onsubmit(pin).catch(() => false);
			busy = false;
			if (!ok) pin = '';
		}
	}

	function onKey(a: Action): boolean {
		if (a === 'enter') void press(keys[index] ?? '0');
		else if (a === 'back') {
			if (pin === '') void oncancel();
			else pin = pin.slice(0, -1);
		} else if (isDirection(a)) {
			index = moveIndex(index, a, keys.length, 3) ?? index; // the focus stays on the pad
		} else return false;
		return true;
	}

	function onkeydown(e: KeyboardEvent) {
		const d = digitKey(e);
		if (d === null || e.altKey || e.ctrlKey || e.metaKey) return;
		const t = e.target;
		if (t instanceof HTMLInputElement || t instanceof HTMLTextAreaElement) return;
		e.preventDefault();
		void press(String(d) as Key);
	}

	const label = (k: Key) => (k === 'cancel' ? 'Cancelar' : k === 'erase' ? '⌫' : k);
</script>

<svelte:window {onkeydown} />

<div class="backdrop">
	<div class="pad" role="dialog" aria-label={title}>
		<p class="title">{title}</p>
		<div class="dots" aria-label={`${pin.length} de ${LENGTH} dígitos`}>
			{#each { length: LENGTH } as _, i (i)}
				<span class="dot" class:on={i < pin.length}></span>
			{/each}
		</div>
		<p class="error" aria-live="polite">{error}</p>
		<div class="keys" use:focusable={{ id, onKey }}>
			{#each keys as k, i (k)}
				<button
					tabindex="-1"
					class="key"
					class:small={k === 'cancel'}
					class:current={i === index}
					aria-label={k === 'erase' ? 'Apagar' : label(k)}
					onpointerenter={() => (index = i)}
					onclick={() => void press(k)}>{label(k)}</button
				>
			{/each}
		</div>
	</div>
</div>

<style>
	.backdrop {
		position: fixed;
		top: 0;
		left: 0;
		width: 100%;
		height: 100%;
		z-index: 10;
		display: flex;
		align-items: center;
		justify-content: center;
		background: rgba(11, 11, 15, 0.92);
	}
	.pad {
		text-align: center;
	}
	.title {
		margin: 0;
		font-size: 2rem;
		font-weight: 600;
	}
	.dots {
		display: flex;
		justify-content: center;
		margin-top: 2rem;
	}
	.dot {
		width: 1.25rem;
		height: 1.25rem;
		margin: 0 0.75rem;
		border: 2px solid rgba(255, 255, 255, 0.6);
		border-radius: 999px;
	}
	.dot.on {
		border-color: rgba(255, 255, 255, 0.92);
		background: rgba(255, 255, 255, 0.92);
	}
	.error {
		min-height: 1.5rem;
		margin: 1rem 0;
		color: #ff8a8a;
		font-size: 1.125rem;
	}
	.keys {
		display: grid;
		grid-template-columns: repeat(3, 5rem);
		grid-gap: 1rem;
		justify-content: center;
		outline: none;
	}
	.key {
		height: 5rem;
		border: 0;
		border-radius: 999px;
		background: rgba(255, 255, 255, 0.1);
		color: rgba(255, 255, 255, 0.92);
		font: inherit;
		font-size: 1.75rem;
		font-weight: 600;
		cursor: pointer;
		transition: transform 200ms ease-out;
	}
	.key.small {
		font-size: 1rem;
	}
	.keys:global([data-focused]) > .key.current {
		background: rgba(255, 255, 255, 0.92);
		color: #0b0b0f;
		transform: scale(1.08);
		box-shadow:
			0 12px 30px rgba(0, 0, 0, 0.5),
			0 0 0 3px #4c8dff;
	}
	/* TV: text of at least 24px at 1080p (AGENTS.md §7). */
	:global(html.tv) .error,
	:global(html.tv) .key.small {
		font-size: 1.5rem;
	}
</style>
