<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';

	import { seriesProgress, setWatched } from '#lib/api/client.ts';
	import type { Episode, SeriesProgress, WatchedTarget } from '#lib/api/types.ts';
	import { focusGroup } from '#lib/platform/nav.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import Card from '#lib/ui/Card.svelte';
	import Grid from '#lib/ui/Grid.svelte';
	import Hero from '#lib/ui/Hero.svelte';
	import PlayButtons from '#lib/ui/PlayButtons.svelte';
	import Row from '#lib/ui/Row.svelte';
	import { episodeLabel, episodeTitle, extraCard, formatDuration, playHref } from '#lib/ui/cards.ts';
	import { seriesHero, seriesPlay } from '#lib/ui/hero.ts';
	import { fraction, remaining } from '#lib/ui/progress.ts';

	const id = $derived(page.params.id ?? '');
	const data = $derived(seriesHero(id));

	/** Selected tab: a season number, or "extras". */
	let tab: number | 'extras' | undefined = $state();
	/** Progress reloaded after a manual mark, by series id. */
	let fresh: Record<string, SeriesProgress> = $state({});

	const seasonName = (n: number) => (n === 0 ? 'Especiais' : `Temporada ${n}`);
	const play = (title: string, e: Episode) => goto(playHref(e.media_id, episodeTitle(title, e)));

	async function mark(seriesId: string, t: WatchedTarget) {
		await setWatched(t);
		fresh[seriesId] = await seriesProgress(seriesId);
	}
</script>

{#await data then h}
	{@const seasons = h.detail.seasons}
	{@const prog = fresh[h.detail.id] ?? h.progress}
	{@const eps = prog?.episodes ?? {}}
	{@const isWatched = (e: Episode) => eps[e.media_id]?.watched === true}
	{@const allWatched = seasons.every((s) => s.episodes.every(isWatched))}
	{@const main = seriesPlay(h.detail, prog)}
	{@const current = tab ?? seasons[0]?.number}
	{@const season = seasons.find((s) => s.number === current)}
	<Hero title={h.title} meta={h.meta} imageId={h.imageId} color={h.color} trailerSrc={h.trailerSrc}>
		{#snippet actions()}
			{#if main}
				<PlayButtons id="series:play" play={main} />
			{/if}
			{#if h.trailer}
				{@const t = h.trailer}
				<Button onclick={() => goto(extraCard(t).href)}>Assistir trailer</Button>
			{/if}
			<Button
				id="series:watched"
				onclick={() => mark(h.detail.id, { series_id: h.detail.id, watched: !allWatched })}
			>
				{allWatched ? '✓ Assistida' : 'Marcar como assistida'}
			</Button>
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
		{@const seasonWatched = season.episodes.every(isWatched)}
		<div class="season-bar" use:focusGroup={{ orientation: 'horizontal' }}>
			<Button
				id={`season:watched:${season.number}`}
				onclick={() => mark(h.detail.id, { season_id: season.id, watched: !seasonWatched })}
			>
				{seasonWatched ? '✓ Temporada assistida' : 'Marcar temporada como assistida'}
			</Button>
		</div>
		{#key season.id}
			<Grid
				id={`episodes:${season.id}`}
				items={season.episodes}
				key={(e: Episode) => e.id}
				minWidth={0}
				onopen={(e: Episode) => play(h.title, e)}
				onside={(e: Episode) => mark(h.detail.id, { media_ids: [e.media_id], watched: !isWatched(e) })}
			>
				{#snippet cell(e: Episode, focused: boolean, side: boolean)}
					{@const p = eps[e.media_id]}
					{@const watched = isWatched(e)}
					{@const part = fraction(p)}
					<div class="episode">
						<div class="thumb">
							<Card
								item={{
									key: e.id,
									imageId: e.id,
									title: h.title,
									color: e.color,
									href: '',
									progress: part,
									watched,
								}}
								current={focused && !side}
								sizes="480px"
							/>
						</div>
						<div class="info">
							<p class="label">{episodeLabel(e)}{e.title ? ` · ${e.title}` : ''}</p>
							<p class="duration">
								{p !== undefined && part !== undefined ? remaining(p) : formatDuration(e.duration_ms)}
							</p>
						</div>
						<button
							class="mark"
							class:on={watched}
							class:kfocus={side}
							tabindex="-1"
							aria-label={watched ? 'Desmarcar como assistido' : 'Marcar como assistido'}
							title={watched ? 'Desmarcar como assistido' : 'Marcar como assistido'}
							onclick={(ev) => {
								ev.stopPropagation();
								void mark(h.detail.id, { media_ids: [e.media_id], watched: !watched });
							}}>✓</button
						>
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
	.season-bar {
		display: flex;
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
	.info {
		flex: 1;
		min-width: 0;
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
	/* Right on a focused episode moves here (Grid onside). */
	.mark {
		flex: none;
		width: 3rem;
		height: 3rem;
		margin: 0 1rem;
		border: 2px solid rgba(255, 255, 255, 0.4);
		border-radius: 999px;
		background: transparent;
		color: rgba(255, 255, 255, 0.4);
		font: inherit;
		font-size: 1.25rem;
		font-weight: 700;
		cursor: pointer;
		transition: transform 200ms ease-out;
	}
	.mark.on {
		border-color: rgba(255, 255, 255, 0.92);
		background: rgba(255, 255, 255, 0.92);
		color: #0b0b0f;
	}
	:global([data-focused]) .mark.kfocus {
		transform: scale(1.08);
		box-shadow:
			0 12px 30px rgba(0, 0, 0, 0.5),
			0 0 0 3px #4c8dff;
	}
	.error {
		margin-top: 20vh;
		text-align: center;
		font-size: 1.25rem;
		color: rgba(255, 255, 255, 0.6);
	}
	/* TV: text of at least 24px at 1080p (AGENTS.md §7). */
	:global(html.tv) .label,
	:global(html.tv) .mark,
	:global(html.tv) .error {
		font-size: 1.5rem;
	}
</style>
