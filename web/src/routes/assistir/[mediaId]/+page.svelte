<script lang="ts">
	import { page } from '$app/state';

	import Player from '#lib/ui/Player.svelte';

	const mediaId = $derived(page.params.mediaId ?? '');
</script>

<!-- A new player per file: the next episode starts with a clean state. -->
{#key mediaId}
	<Player
		{mediaId}
		title={page.url.searchParams.get('title') ?? ''}
		fromStart={page.url.searchParams.has('inicio')}
	/>
{/key}
