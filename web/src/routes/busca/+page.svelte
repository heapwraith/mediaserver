<script lang="ts">
	import { listMovies, listSeries } from '#lib/api/client.ts';
	import { focusable } from '#lib/platform/nav.svelte.ts';
	import Row from '#lib/ui/Row.svelte';
	import { movieCard, seriesCard, type CardItem } from '#lib/ui/cards.ts';

	const DEBOUNCE_MS = 300;
	const LIMIT = 50;

	let q = $state('');
	let results: { movies: CardItem[]; series: CardItem[]; animes: CardItem[] } | undefined =
		$state();
	let gen = 0;

	async function search(text: string) {
		const mine = ++gen;
		if (text.trim() === '') {
			results = undefined;
			return;
		}
		const p = { q: text, limit: LIMIT };
		const [m, s, a] = await Promise.all([
			listMovies(p),
			listSeries('series', p),
			listSeries('anime', p),
		]);
		if (mine !== gen) return; // an older answer arriving late
		results = {
			movies: m.items.map(movieCard),
			series: s.items.map(seriesCard),
			animes: a.items.map(seriesCard),
		};
	}

	// Debounce: search once typing pauses.
	$effect(() => {
		const text = q;
		const timer = setTimeout(() => void search(text).catch(() => (results = undefined)), DEBOUNCE_MS);
		return () => clearTimeout(timer);
	});

	const empty = $derived(
		results !== undefined &&
			results.movies.length + results.series.length + results.animes.length === 0,
	);
</script>

<div class="search">
	<input
		type="search"
		placeholder="Filmes, séries e animes"
		aria-label="Buscar"
		autocomplete="off"
		bind:value={q}
		use:focusable={{ id: 'search:input' }}
	/>
</div>

{#if empty}
	<p class="note">Nada encontrado para “{q}”.</p>
{:else if results !== undefined}
	<Row id="search:movies" title="Filmes" items={results.movies} />
	<Row id="search:series" title="Séries" items={results.series} />
	<Row id="search:animes" title="Animes" items={results.animes} />
{/if}

<style>
	.search {
		margin: 2rem 0 2.5rem;
	}
	input {
		width: 100%;
		max-width: 48rem;
		height: 4rem;
		padding: 0 1.5rem;
		border: 0;
		border-radius: 999px;
		background: rgba(255, 255, 255, 0.1);
		color: rgba(255, 255, 255, 0.92);
		font: inherit;
		font-size: 1.5rem;
		outline: none;
	}
	input:global([data-focused]) {
		box-shadow: 0 0 0 3px #4c8dff;
	}
	.note {
		font-size: 1.25rem;
		color: rgba(255, 255, 255, 0.6);
	}
	/* TV: text of at least 24px at 1080p (AGENTS.md §7). */
	:global(html.tv) .note {
		font-size: 1.5rem;
	}
</style>
