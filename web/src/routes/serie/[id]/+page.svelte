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
	import Icon from '#lib/ui/Icon.svelte';
	import PlayButtons from '#lib/ui/PlayButtons.svelte';
	import Row from '#lib/ui/Row.svelte';
	import {
		episodeLabel,
		episodeTitle,
		extraCard,
		formatDuration,
		playHref,
		resolution,
	} from '#lib/ui/cards.ts';
	import { seriesHero, seriesPlay } from '#lib/ui/hero.ts';
	import { fraction, remaining } from '#lib/ui/progress.ts';
	import ErrorNotice from '#lib/ui/ErrorNotice.svelte';

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
	<Hero
		compact
		title={h.title}
		meta={h.meta}
		badges={h.badges}
		imageId={h.imageId}
		color={h.color}
		trailerSrc={h.trailerSrc}
	>
		{#snippet actions()}
			{#if main}
				<PlayButtons id="series:play" play={main} />
			{/if}
			{#if h.trailer}
				{@const t = h.trailer}
				<Button icon="play" onclick={() => goto(extraCard(t).href)}>Trailer</Button>
			{/if}
			<Button
				id="series:watched"
				icon={allWatched ? 'check' : undefined}
				onclick={() => mark(h.detail.id, { series_id: h.detail.id, watched: !allWatched })}
			>
				{allWatched ? 'Assistida' : 'Marcar como assistida'}
			</Button>
		{/snippet}
	</Hero>

	{@const seasonWatched = season?.episodes.every(isWatched) ?? false}
	<!-- Seasons, extras and "mark the season" share one line. -->
	<div class="tabs" use:focusGroup={{ id: 'tabs', orientation: 'horizontal' }}>
		{#each seasons as s (s.id)}
			<Button selected={current === s.number} id={`tab:${s.number}`} onclick={() => (tab = s.number)}>
				{seasonName(s.number)}
			</Button>
		{/each}
		{#if h.extras.length > 0}
			<Button selected={current === 'extras'} id="tab:extras" onclick={() => (tab = 'extras')}>Extras</Button>
		{/if}
		{#if current !== 'extras' && season !== undefined}
			<span class="push"></span>
			<Button
				id={`season:watched:${season.number}`}
				icon={seasonWatched ? 'check' : undefined}
				onclick={() => mark(h.detail.id, { season_id: season.id, watched: !seasonWatched })}
			>
				{seasonWatched ? 'Temporada assistida' : 'Marcar temporada como assistida'}
			</Button>
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
				onside={(e: Episode) => mark(h.detail.id, { media_ids: [e.media_id], watched: !isWatched(e) })}
			>
				{#snippet cell(e: Episode, focused: boolean, side: boolean)}
					{@const p = eps[e.media_id]}
					{@const watched = isWatched(e)}
					{@const part = fraction(p)}
					{@const n = e.number ?? e.absolute_number}
					<div class="episode" class:focused={focused && !side}>
						<span class="n">{n ?? ''}</span>
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
								sizes="480px"
							/>
						</div>
						<div class="info">
							<p class="label">
								{n === undefined ? episodeLabel(e) : `Episódio ${episodeLabel(e).replace('Ep. ', '')}`}{e.title
									? ` · ${e.title}`
									: ''}
							</p>
							<p class="state">
								{#if watched}Visto{:else if p !== undefined && part !== undefined}<span class="go"
										>Continuar</span
									>
									· {remaining(p)}{:else}{resolution(e.height, e.width)}{/if}
							</p>
						</div>
						<span class="duration">{formatDuration(e.duration_ms)}</span>
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
							}}><Icon name="check" size="1.5rem" /></button
						>
					</div>
				{/snippet}
			</Grid>
		{/key}
	{/if}
{:catch e}
	<ErrorNotice error={e} />
{/await}

<style>
	.tabs {
		display: flex;
		align-items: center;
		margin: 0 0 1.5rem;
	}
	.tabs > :global(*) {
		margin-right: 0.75rem;
	}
	.push {
		flex: 1;
	}
	/* Telão (ADR 013): big number, picture, state, duration; the whole line takes the focus. */
	.episode {
		display: flex;
		align-items: center;
		padding: 1.125rem 1.375rem;
		border-radius: 14px;
		transition:
			transform 200ms cubic-bezier(0.2, 0.8, 0.2, 1),
			background-color 200ms ease-out,
			box-shadow 200ms ease-out;
	}
	.episode.focused {
		background: var(--panel);
		transform: scale(1.02);
		box-shadow: 0 0 0 4px var(--ring);
	}
	.n {
		flex: none;
		width: 5.5rem;
		font-family: var(--font-display);
		font-size: 4rem;
		line-height: 1;
		text-align: center;
		color: var(--text-3);
	}
	.focused .n {
		color: var(--text);
	}
	.thumb {
		flex: none;
		width: 20rem;
		margin-right: 2rem;
	}
	/* The episode picture shows no text: it is beside it. */
	.thumb :global(.label),
	.thumb :global(.shade) {
		display: none;
	}
	.info {
		flex: 1;
		min-width: 0;
	}
	.label {
		margin: 0;
		font-size: 1.875rem;
		font-weight: 800;
	}
	.state {
		margin: 0.375rem 0 0;
		font-size: 1.375rem;
		color: var(--text-2);
	}
	.go {
		color: var(--accent);
		font-weight: 700;
	}
	.duration {
		flex: none;
		margin: 0 1.5rem;
		font-size: 1.375rem;
		font-weight: 700;
		color: var(--text-2);
	}
	/* Right on a focused episode moves here (Grid onside). */
	.mark {
		flex: none;
		display: flex;
		align-items: center;
		justify-content: center;
		width: 3.5rem;
		height: 3.5rem;
		border: 2px solid var(--text-3);
		border-radius: 999px;
		background: transparent;
		color: var(--text-3);
		cursor: pointer;
		transition: transform 200ms ease-out;
	}
	.mark.on {
		border-color: var(--text);
		background: var(--text);
		color: #000;
	}
	:global([data-focused]) .mark.kfocus {
		transform: scale(1.1);
		box-shadow:
			0 12px 30px rgba(0, 0, 0, 0.5),
			0 0 0 4px var(--ring);
	}
	/* TV: text of at least 24px at 1080p (AGENTS.md §7). */
	:global(html.tv) .state,
	:global(html.tv) .duration {
		font-size: 1.5rem;
	}
</style>
