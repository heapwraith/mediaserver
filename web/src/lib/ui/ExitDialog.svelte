<script lang="ts">
	import { onMount } from 'svelte';

	import type { Action } from '../platform/keys';
	import { focusId, nav } from '../platform/nav.svelte';
	import Button from './Button.svelte';

	// Voltar on Início of a TV app (AGENTS.md §7): asks before leaving.
	let { onexit, oncancel }: { onexit: () => void; oncancel: () => void } = $props();

	const LEAVE = 'exit:leave';
	const STAY = 'exit:stay';
	const before = nav.focused;

	onMount(() => {
		focusId(LEAVE);
		return () => {
			if (before !== '') focusId(before);
		};
	});

	/** Keeps every key inside the dialog: arrows only switch between the two buttons. */
	function onKey(a: Action): boolean {
		if (a === 'left' || a === 'right') focusId(nav.focused === LEAVE ? STAY : LEAVE);
		else if (a === 'back') oncancel();
		else if (a === 'enter') return false; // the button's click
		return true;
	}
</script>

<div class="scrim" role="dialog" aria-modal="true" aria-labelledby="exit-title">
	<div class="box">
		<h2 id="exit-title">Sair do mediaserver?</h2>
		<div class="actions">
			<Button primary id={LEAVE} {onKey} onclick={onexit}>Sair</Button>
			<Button id={STAY} {onKey} onclick={oncancel}>Cancelar</Button>
		</div>
	</div>
</div>

<style>
	.scrim {
		position: fixed;
		top: 0;
		right: 0;
		bottom: 0;
		left: 0;
		z-index: 100;
		display: flex;
		align-items: center;
		justify-content: center;
		/* No blur (AGENTS.md §7): a dark veil is enough. */
		background: rgba(11, 11, 15, 0.85);
	}
	.box {
		padding: 3rem 4rem;
		border-radius: 12px;
		background: #1e1e26;
		text-align: center;
	}
	h2 {
		margin: 0 0 2.5rem;
		font-size: 2.25rem;
		font-weight: 600;
	}
	.actions > :global(* + *) {
		margin-left: 1.5rem;
	}
</style>
