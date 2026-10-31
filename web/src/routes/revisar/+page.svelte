<script lang="ts">
	import {
		correctFile,
		getSeries,
		listSeries,
		mergeSeries,
		reviewQueue,
		splitSeries,
	} from '#lib/api/client.ts';
	import type { CorrectionKind, Episode, ReviewItem, Series } from '#lib/api/types.ts';
	import { focusGroup, focusable } from '#lib/platform/nav.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import Grid from '#lib/ui/Grid.svelte';
	import { episodeLabel } from '#lib/ui/cards.ts';

	const reasons: Record<string, string> = {
		no_year: 'Sem ano nem episódio no nome',
		no_title: 'Sem título reconhecível',
		not_recognized: 'Nome não reconhecido',
		folder_conflict: 'A pasta indica outro tipo',
		duplicate_episode: 'Episódio repetido',
		episode_without_season: 'Episódio sem temporada',
		season_without_episode: 'Temporada sem episódio',
		extra_without_owner: 'Extra sem filme ou série na pasta',
	};
	const kinds: { kind: CorrectionKind; label: string }[] = [
		{ kind: 'movie', label: 'Filme' },
		{ kind: 'series', label: 'Série' },
		{ kind: 'anime', label: 'Anime' },
		{ kind: 'ignored', label: 'Ignorar' },
	];

	let queue: ReviewItem[] = $state([]);
	let message = $state('');

	// Editor of one file.
	let editing: ReviewItem | undefined = $state();
	let form = $state({ kind: 'movie' as CorrectionKind, title: '', year: '', season: '', episode: '' });

	// Merge / split.
	let allSeries: Series[] = $state([]);
	let picked: string[] = $state([]);
	let splitting: { series: Series; episodes: Episode[]; files: string[]; title: string } | undefined =
		$state();

	const fail = (e: unknown) => (message = `Erro: ${e instanceof Error ? e.message : String(e)}`);

	async function reload() {
		try {
			const [q, s, a] = await Promise.all([
				reviewQueue(),
				listSeries('series', { limit: 500 }),
				listSeries('anime', { limit: 500 }),
			]);
			queue = q;
			allSeries = [...s.items, ...a.items].sort((x, y) => x.title.localeCompare(y.title, 'pt-BR'));
			picked = picked.filter((id) => allSeries.some((x) => x.id === id));
		} catch (e) {
			fail(e);
		}
	}
	void reload();

	function edit(item: ReviewItem) {
		editing = item;
		const g = item.guess;
		form = {
			kind: g.kind === 'series' || g.kind === 'anime' || g.kind === 'movie' ? g.kind : 'movie',
			title: g.title ?? '',
			year: g.year === undefined ? '' : String(g.year),
			season: g.season === undefined ? '' : String(g.season),
			episode: String(g.episode ?? g.absolute ?? ''),
		};
	}

	const num = (s: string) => (s.trim() === '' ? undefined : Number(s));

	async function save() {
		if (editing === undefined) return;
		const episodic = form.kind === 'series' || form.kind === 'anime';
		try {
			await correctFile(editing.id, {
				kind: form.kind,
				title: form.kind === 'ignored' ? undefined : form.title.trim(),
				year: form.kind === 'movie' ? num(form.year) : undefined,
				season: episodic ? (num(form.season) ?? 1) : undefined,
				episode: episodic ? num(form.episode) : undefined,
			});
			message = `“${editing.rel_path}” corrigido.`;
			editing = undefined;
			await reload();
		} catch (e) {
			fail(e);
		}
	}

	function pick(s: Series) {
		picked = picked.includes(s.id) ? picked.filter((id) => id !== s.id) : [...picked, s.id];
	}

	async function merge() {
		const [target, ...sources] = picked;
		if (target === undefined || sources.length === 0) return;
		try {
			await mergeSeries(sources, target);
			message = 'Séries mescladas.';
			picked = [];
			await reload();
		} catch (e) {
			fail(e);
		}
	}

	async function startSplit() {
		const s = allSeries.find((x) => x.id === picked[0]);
		if (s === undefined) return;
		try {
			const d = await getSeries(s.id);
			splitting = { series: s, episodes: d.seasons.flatMap((x) => x.episodes), files: [], title: '' };
		} catch (e) {
			fail(e);
		}
	}

	function toggleFile(e: Episode) {
		if (splitting === undefined) return;
		const f = splitting.files;
		splitting.files = f.includes(e.media_id) ? f.filter((x) => x !== e.media_id) : [...f, e.media_id];
	}

	async function split() {
		if (splitting === undefined) return;
		try {
			await splitSeries(splitting.series.id, splitting.files, splitting.title.trim());
			message = 'Episódios separados numa série nova.';
			splitting = undefined;
			picked = [];
			await reload();
		} catch (e) {
			fail(e);
		}
	}
</script>

<h1>Revisar</h1>
{#if message}<p class="message">{message}</p>{/if}

{#if editing !== undefined}
	<section class="panel">
		<h2>{editing.rel_path}</h2>
		<p class="reason">{reasons[editing.reason] ?? editing.reason}</p>
		<div class="bar" use:focusGroup={{ orientation: 'horizontal' }}>
			{#each kinds as k (k.kind)}
				<Button selected={form.kind === k.kind} onclick={() => (form.kind = k.kind)}>{k.label}</Button>
			{/each}
		</div>
		{#if form.kind !== 'ignored'}
			<div class="fields">
				<label>
					Título
					<input bind:value={form.title} use:focusable autocomplete="off" />
				</label>
				{#if form.kind === 'movie'}
					<label>
						Ano
						<input bind:value={form.year} inputmode="numeric" use:focusable autocomplete="off" />
					</label>
				{:else}
					<label>
						Temporada
						<input bind:value={form.season} inputmode="numeric" use:focusable autocomplete="off" />
					</label>
					<label>
						Episódio
						<input bind:value={form.episode} inputmode="numeric" use:focusable autocomplete="off" />
					</label>
				{/if}
			</div>
		{/if}
		<div class="bar" use:focusGroup={{ orientation: 'horizontal' }}>
			<Button primary onclick={save}>Salvar</Button>
			<Button onclick={() => (editing = undefined)}>Cancelar</Button>
		</div>
	</section>
{:else if splitting !== undefined}
	{@const sp = splitting}
	<section class="panel">
		<h2>Separar episódios de “{sp.series.title}”</h2>
		<label>
			Título da série nova
			<input bind:value={sp.title} use:focusable autocomplete="off" />
		</label>
		<Grid
			id="split:episodes"
			items={sp.episodes}
			key={(e: Episode) => e.id}
			minWidth={16}
			onopen={toggleFile}
		>
			{#snippet cell(e: Episode, current: boolean)}
				<div class="chip" class:current class:on={sp.files.includes(e.media_id)}>
					{episodeLabel(e) || 'Episódio'}{e.title ? ` · ${e.title}` : ''}
				</div>
			{/snippet}
		</Grid>
		<div class="bar" use:focusGroup={{ orientation: 'horizontal' }}>
			<Button primary onclick={split}>Separar {sp.files.length} episódio(s)</Button>
			<Button onclick={() => (splitting = undefined)}>Cancelar</Button>
		</div>
	</section>
{:else}
	<h2>Arquivos ({queue.length})</h2>
	{#if queue.length === 0}
		<p class="note">Nada para revisar.</p>
	{/if}
	<Grid id="review:queue" items={queue} key={(i: ReviewItem) => i.id} minWidth={0} onopen={edit}>
		{#snippet cell(item: ReviewItem, current: boolean)}
			<div class="chip" class:current>
				<strong>{item.rel_path}</strong>
				<span>{reasons[item.reason] ?? item.reason}</span>
			</div>
		{/snippet}
	</Grid>

	<h2>Mesclar ou separar séries</h2>
	<p class="note">
		Selecione séries com Enter. Mesclar junta todas na primeira escolhida; Separar move episódios de
		uma série para uma nova.
	</p>
	<Grid id="review:series" items={allSeries} key={(s: Series) => s.id} minWidth={18} onopen={pick}>
		{#snippet cell(s: Series, current: boolean)}
			<div class="chip" class:current class:on={picked.includes(s.id)}>
				{#if picked.indexOf(s.id) === 0}<em>destino · </em>{/if}{s.title}
				<span>{s.kind === 'anime' ? 'Anime' : 'Série'} · {s.episodes} ep.</span>
			</div>
		{/snippet}
	</Grid>
	<div class="bar" use:focusGroup={{ orientation: 'horizontal' }}>
		{#if picked.length >= 2}
			<Button primary onclick={merge}>Mesclar {picked.length} séries</Button>
		{/if}
		{#if picked.length === 1}
			<Button onclick={startSplit}>Separar episódios…</Button>
		{/if}
	</div>
{/if}

<style>
	h1 {
		margin: 2rem 0 1rem;
		font-size: 2.5rem;
	}
	h2 {
		margin: 2rem 0 0.5rem;
		font-size: 1.5rem;
		word-break: break-all;
	}
	.message {
		font-size: 1.125rem;
		color: #4c8dff;
	}
	.note,
	.reason {
		color: rgba(255, 255, 255, 0.6);
	}
	.panel {
		max-width: 60rem;
	}
	.bar {
		display: flex;
		margin: 1.5rem 0;
	}
	.bar > :global(* + *) {
		margin-left: 1rem;
	}
	.fields {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(14rem, 1fr));
		grid-gap: 1rem;
	}
	label {
		display: block;
		margin: 1rem 0;
		color: rgba(255, 255, 255, 0.6);
	}
	input {
		display: block;
		width: 100%;
		height: 3.25rem;
		margin-top: 0.5rem;
		padding: 0 1.25rem;
		border: 0;
		border-radius: 12px;
		background: rgba(255, 255, 255, 0.1);
		color: rgba(255, 255, 255, 0.92);
		font: inherit;
		font-size: 1.25rem;
		outline: none;
	}
	input:global([data-focused]) {
		box-shadow: 0 0 0 3px #4c8dff;
	}
	.chip {
		padding: 1rem 1.25rem;
		border-radius: 12px;
		background: #1e1e26;
		transition: transform 200ms ease-out;
	}
	.chip strong,
	.chip span {
		display: block;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.chip span {
		margin-top: 0.25rem;
		color: rgba(255, 255, 255, 0.6);
	}
	.chip.on {
		background: #284a8a;
	}
	:global([data-focused]) .chip.current {
		transform: scale(1.03);
		box-shadow: 0 0 0 3px rgba(255, 255, 255, 0.92);
	}
	/* TV: text of at least 24px at 1080p (AGENTS.md §7). */
	:global(html.tv) .message,
	:global(html.tv) input {
		font-size: 1.5rem;
	}
</style>
