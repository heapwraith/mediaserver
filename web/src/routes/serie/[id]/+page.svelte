<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';

	import type { Episode } from '#lib/api/types.ts';
	import { focusGroup } from '#lib/platform/nav.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import Card from '#lib/ui/Card.svelte';
	import Grid from '#lib/ui/Grid.svelte';
	import Hero from '#lib/ui/Hero.svelte';
	import Row from '#lib/ui/Row.svelte';
	import { episodeLabel, extraCard, formatDuration, playHref } from '#lib/ui/cards.ts';
	import { seriesHero } from '#lib/ui/hero.ts';

	const id = $derived(page.params.id ?? '');
	const data = $derived(seriesHero(id));

	/** Selected tab: a season number, or "extras". */
	let tab: number | 'extras' | undefined = $state();

	const seasonName = (n: number) => (n === 0 ? 'Especiais' : `Temporada ${n}`);
	const play = (title: string, e: Episode) => goto(playHref(e.media_id, `${title} · ${episodeLabel(e)}`));
</script>

{#await data then h}
	{@const seasons = h.detail.seasons}
	{@const current = tab ?? seasons[0]?.number}
	{@const season = seasons.find((s) => s.number === current)}
	<Hero title={h.title} meta={h.meta} imageId={h.imageId} color={h.color} trailerSrc={h.trailerSrc}>
		{#snippet actions()}
			{#if h.play}
				<Button primary id="series:play" onclick={() => goto(h.play ?? '/')}>▶ Assistir</Button>
			{/if}
			{#if h.trailer}
				{@const t = h.trailer}
				<Button onclick={() => goto(extraCard(t).href)}>Assistir trailer</Button>
			{/if}
		{/snippet}
	</Hero>

	<div class="tabs" use:focusGroup={{ id: 'tabs', orientation: 'horizontal' }}>
		{#each seasons as s (s.id)}
			<Button selected={current === s.number} id={`tab:${s.number}`} onclick={() => (tab = s.number)}>
				{seasonName(s.number)}
			</Button>
		{/each}
		{#if h.extras.length > 0}
			<Button selected={current === 'extras'} id="tab:extras" onclick={() => (tab = 'extras')}>Extras</Button>
		{/if}
	</div>

	{#if current === 'extras'}
		<Row id="extras" title="Extras" items={h.extras.map(extraCard)} />
	{:else if season !== undefined}
		{#key season.id}
			<Grid
				id={`episodes:${season.id}`}
				items={season.episodes}
				key={(e: Episode) => e.id}
				minWidth={0}
				onopen={(e: Episode) => play(h.title, e)}
			>
				{#snippet cell(e: Episode, focused: boolean)}
					<div class="episode">
						<div class="thumb">
							<Card
								item={{ key: e.id, imageId: e.id, title: h.title, color: e.color, href: '' }}
								current={focused}
								sizes="480px"
							/>
						</div>
						<div class="info">
							<p class="label">{episodeLabel(e)}{e.title ? ` · ${e.title}` : ''}</p>
							<p class="duration">{formatDuration(e.duration_ms)}</p>
							<!-- Barra de progresso: Sprint 4. -->
						</div>
					</div>
				{/snippet}
			</Grid>
		{/key}
	{/if}
{:catch e}
	<p class="error">Série não encontrada ({e instanceof Error ? e.message : e}).</p>
{/await}

<style>
	.tabs {
		display: flex;
		flex-wrap: wrap;
		margin-bottom: 1rem;
	}
	.tabs > :global(*) {
		margin: 0 0.75rem 0.75rem 0;
	}
	.episode {
		display: flex;
		align-items: center;
	}
	.thumb {
		flex: none;
		width: 16rem;
		margin-right: 1.5rem;
	}
	/* The episode card shows only the image; the text is beside it. */
	.thumb :global(.title),
	.thumb :global(.subtitle) {
		display: none;
	}
	.label {
		margin: 0;
		font-size: 1.25rem;
		font-weight: 600;
	}
	.duration {
		margin: 0.5rem 0 0;
		color: rgba(255, 255, 255, 0.6);
	}
	.error {
		margin-top: 20vh;
		text-align: center;
		font-size: 1.25rem;
		color: rgba(255, 255, 255, 0.6);
	}
</style>
