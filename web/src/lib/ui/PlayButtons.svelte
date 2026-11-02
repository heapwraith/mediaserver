<script lang="ts">
	import { goto } from '$app/navigation';

	import Button from './Button.svelte';
	import type { PlayAction } from './hero';

	/** "Continuar · faltam X" + "Do início", or "Assistir" (with the play icon). */
	let { play, id }: { play: PlayAction; id: string } = $props();
</script>

<Button primary icon="play" {id} onclick={() => goto(play.href)}>{play.label}</Button>
{#if play.restart !== undefined}
	{@const restart = play.restart}
	<Button icon="restart" id={`${id}:restart`} onclick={() => goto(restart)}>Do início</Button>
{/if}
