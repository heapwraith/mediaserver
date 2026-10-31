<script lang="ts">
	import { goto } from '$app/navigation';

	import { getHome, getMovie, listMovies, listSeries } from '#lib/api/client.ts';
	import type { Home, HomeItem } from '#lib/api/types.ts';
	import Button from '#lib/ui/Button.svelte';
	import Hero from '#lib/ui/Hero.svelte';
	import PlayButtons from '#lib/ui/PlayButtons.svelte';
	import Row from '#lib/ui/Row.svelte';
	import { extraCard, homeCard, movieCard, seriesCard, type CardItem } from '#lib/ui/cards.ts';
	import { movieHero, seriesHero, type HeroData } from '#lib/ui/hero.ts';

	const recent = { sort: 'added', order: 'desc', limit: 20 } as const;
	const noRows: Home = { continue: [], next: [] };

	/** The hero of something the profile is watching. */
	const itemHero = async (it: HomeItem): Promise<HeroData> =>
		it.kind === 'movie' ? movieHero(await getMovie(it.parent_id)) : seriesHero(it.parent_id);

	async function load() {
		const [movies, series, animes, rows] = await Promise.all([
			listMovies(recent),
			listSeries('series', recent),
			listSeries('anime', recent),
			getHome().catch(() => noRows),
		]);
		const added: { at: number; card: CardItem; hero: () => Promise<HeroData> }[] = [
			...movies.items.map((m) => ({ at: m.added_at, card: movieCard(m), hero: () => movieHero(m) })),
			...[...series.items, ...animes.items].map((s) => ({
				at: s.added_at,
				card: seriesCard(s),
				hero: () => seriesHero(s.id),
			})),
		].sort((a, b) => b.at - a.at);
		// The hero: what the profile is watching, else the latest addition.
		const watching = rows.continue[0];
		const hero = watching === undefined ? added[0]?.hero : () => itemHero(watching);
		return {
			hero: await hero?.().catch(() => added[0]?.hero()),
			continue: rows.continue.map(homeCard),
			next: rows.next.map(homeCard),
			added: added.slice(0, 20).map((a) => a.card),
			movies: movies.items.map(movieCard),
			series: series.items.map(seriesCard),
			animes: animes.items.map(seriesCard),
		};
	}
	const data = load();
</script>

{#await data then d}
	{#if d.hero !== undefined}
		{@const hero = d.hero}
		<Hero title={hero.title} meta={hero.meta} imageId={hero.imageId} color={hero.color}>
			{#snippet actions()}
				{#if hero.play}
					<PlayButtons id="home:play" play={hero.play} />
				{/if}
				{#if hero.trailer}
					{@const t = hero.trailer}
					<Button onclick={() => goto(extraCard(t).href)}>
						Trailer
					</Button>
				{/if}
				<Button onclick={() => goto(hero.details)}>Detalhes</Button>
			{/snippet}
		</Hero>
		<Row id="continue" title="Continuar assistindo" items={d.continue} />
		<Row id="next" title="Próximos episódios" items={d.next} />
		<Row id="added" title="Adicionados recentemente" items={d.added} />
		<Row id="movies" title="Filmes" items={d.movies} />
		<Row id="series" title="Séries" items={d.series} />
		<Row id="animes" title="Animes" items={d.animes} />
	{:else}
		<div class="empty">
			<h1>Nenhuma mídia ainda</h1>
			<p>Coloque filmes, séries ou animes na pasta de mídias. Eles aparecem aqui sozinhos.</p>
			<Button primary onclick={() => goto('/configuracoes')}>Configurações</Button>
		</div>
	{/if}
{:catch e}
	<p class="error">Não foi possível carregar a biblioteca: {e instanceof Error ? e.message : e}</p>
{/await}

<style>
	.empty {
		padding-top: 20vh;
		text-align: center;
	}
	.empty h1 {
		margin: 0;
		font-size: 2.5rem;
	}
	.empty p,
	.error {
		margin: 1rem 0 2rem;
		font-size: 1.25rem;
		color: rgba(255, 255, 255, 0.6);
	}
</style>
