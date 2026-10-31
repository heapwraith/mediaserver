<script lang="ts">
	import { goto } from '$app/navigation';

	import Button from './Button.svelte';
	import type { PlayAction } from './hero';

	/** "▶ Continuar · faltam X" + "Do início", or "▶ Assistir". */
	let { play, id }: { play: PlayAction; id: string } = $props();
</script>

<Button primary {id} onclick={() => goto(play.href)}>{play.label}</Button>
{#if play.restart !== undefined}
	{@const restart = play.restart}
	<Button id={`${id}:restart`} onclick={() => goto(restart)}>Do início</Button>
{/if}
