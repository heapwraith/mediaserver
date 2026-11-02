<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';

	import { getMovie, getProgress, setWatched } from '#lib/api/client.ts';
	import type { Progress } from '#lib/api/types.ts';
	import Button from '#lib/ui/Button.svelte';
	import Hero from '#lib/ui/Hero.svelte';
	import PlayButtons from '#lib/ui/PlayButtons.svelte';
	import Row from '#lib/ui/Row.svelte';
	import { collectionLabel, extraCard } from '#lib/ui/cards.ts';
	import { moviePlay, movieHero } from '#lib/ui/hero.ts';
	import ErrorNotice from '#lib/ui/ErrorNotice.svelte';

	const data = $derived(
		getMovie(page.params.id ?? '').then(async (m) => ({ movie: m, hero: await movieHero(m) })),
	);

	/** Progress after a manual mark, by media id (else the one loaded with the page). */
	let marked: Record<string, Progress> = $state({});

	async function toggleWatched(mediaId: string, watched: boolean) {
		await setWatched({ media_ids: [mediaId], watched: !watched });
		marked[mediaId] = await getProgress(mediaId);
	}
</script>

{#await data then { movie, hero: h }}
	{@const progress = marked[movie.media_id] ?? h.progress}
	{@const watched = progress?.watched === true}
	<Hero title={h.title} meta={h.meta} imageId={h.imageId} color={h.color} trailerSrc={h.trailerSrc}>
		{#snippet actions()}
			<PlayButtons id="movie:play" play={moviePlay(movie, progress)} />
			{#if h.trailer}
				{@const t = h.trailer}
				<Button onclick={() => goto(extraCard(t).href)}>Assistir trailer</Button>
			{/if}
			<Button id="movie:watched" onclick={() => toggleWatched(movie.media_id, watched)}>
				{watched ? '✓ Assistido' : 'Marcar como assistido'}
			</Button>
			{#if movie.collection}
				{@const c = movie.collection}
				<Button onclick={() => goto(`/colecao/${c.id}`)}>{collectionLabel(c.name)}</Button>
			{/if}
		{/snippet}
	</Hero>
	<Row id="extras" title="Extras" items={h.extras.map(extraCard)} />
{:catch e}
	<ErrorNotice error={e} />
{/await}
