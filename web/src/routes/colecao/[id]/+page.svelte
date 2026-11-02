<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';

	import { getCollection } from '#lib/api/client.ts';
	import Card from '#lib/ui/Card.svelte';
	import ErrorNotice from '#lib/ui/ErrorNotice.svelte';
	import Grid from '#lib/ui/Grid.svelte';
	import { movieCard, type CardItem } from '#lib/ui/cards.ts';

	const data = $derived(getCollection(page.params.id ?? ''));
</script>

{#await data then c}
	<h1>{c.name}</h1>
	<Grid
		id={`grid:collection:${c.id}`}
		items={c.movies.map(movieCard)}
		key={(item: CardItem) => item.key}
		onopen={(item: CardItem) => goto(item.href)}
	>
		{#snippet cell(item: CardItem, current: boolean)}
			<Card {item} {current} />
		{/snippet}
	</Grid>
{:catch e}
	<ErrorNotice error={e} />
{/await}

<style>
	h1 {
		margin: 3rem 0 1.5rem;
		font-size: 6rem;
		line-height: 0.9;
	}
</style>
