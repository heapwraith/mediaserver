<script lang="ts">
	import { goto } from '$app/navigation';

	import { listMovies, listSeries, type ListParams } from '../api/client';
	import { focusGroup } from '../platform/nav.svelte';
	import Button from './Button.svelte';
	import Card from './Card.svelte';
	import ErrorNotice from './ErrorNotice.svelte';
	import Grid from './Grid.svelte';
	import { movieCard, seriesCard, type CardItem } from './cards';

	let { kind, title }: { kind: 'movie' | 'series' | 'anime'; title: string } = $props();

	const PAGE = 100;
	const sorts = [
		{ sort: 'title', order: 'asc', label: 'A–Z' },
		{ sort: 'added', order: 'desc', label: 'Recentes' },
		{ sort: 'year', order: 'desc', label: 'Ano' },
	] as const;

	let sortIdx = $state(0);
	let items: CardItem[] = $state([]);
	let total = $state(0);
	let loading = $state(false);
	let gen = 0; // ignores answers of a previous sort
	let error: unknown = $state();

	async function fetchPage(offset: number) {
		const s = sorts[sortIdx] ?? sorts[0];
		const p: ListParams = { sort: s.sort, order: s.order, offset, limit: PAGE };
		if (kind === 'movie') {
			const r = await listMovies(p);
			return { total: r.total, items: r.items.map(movieCard) };
		}
		const r = await listSeries(kind, p);
		return { total: r.total, items: r.items.map(seriesCard) };
	}

	async function more(reset = false) {
		if (!reset && (loading || items.length >= total)) return;
		loading = true;
		const mine = reset ? ++gen : gen;
		try {
			const r = await fetchPage(reset ? 0 : items.length);
			if (mine !== gen) return;
			total = r.total;
			items = reset ? r.items : [...items, ...r.items];
			error = undefined;
		} catch (e) {
			error = e;
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		void sortIdx;
		void more(true);
	});
</script>

<div class="head">
	<h1>{title} <span>{total}</span></h1>
	<div class="sorts" use:focusGroup={{ orientation: 'horizontal' }}>
		{#each sorts as s, i (s.label)}
			<Button selected={i === sortIdx} onclick={() => (sortIdx = i)}>{s.label}</Button>
		{/each}
	</div>
</div>
{#if error !== undefined}<ErrorNotice {error} onretry={() => void more(true)} />{/if}
{#if error === undefined && total === 0 && !loading}<p class="note">Nada por aqui ainda.</p>{/if}

<Grid id={`grid:${kind}`} {items} key={(c) => c.key} onopen={(c) => goto(c.href)} onend={() => more()}>
	{#snippet cell(item, current)}
		<Card {item} {current} />
	{/snippet}
</Grid>

<style>
	.head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin: 2rem 0 1rem;
	}
	h1 {
		margin: 0;
		font-size: 2.5rem;
	}
	h1 span {
		font-size: 1.25rem;
		font-weight: 400;
		color: rgba(255, 255, 255, 0.4);
	}
	.sorts {
		display: flex;
	}
	.sorts > :global(* + *) {
		margin-left: 0.75rem;
	}
	.note {
		color: rgba(255, 255, 255, 0.6);
		font-size: 1.25rem;
	}
	/* TV: text of at least 24px at 1080p (AGENTS.md §7). */
	:global(html.tv) h1 span,
	:global(html.tv) .note {
		font-size: 1.5rem;
	}
</style>
