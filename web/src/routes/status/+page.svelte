<script lang="ts">
	import { onDestroy } from 'svelte';

	import { serverStatus } from '#lib/api/client.ts';
	import type { ServerStatus } from '#lib/api/types.ts';
	import ErrorNotice from '#lib/ui/ErrorNotice.svelte';
	import { bytes, duration, encoderName } from '#lib/ui/status.ts';

	// Server health at a glance (Sprint 8): CPU, memory, conversions, disk.
	const POLL_MS = 2000;

	let s: ServerStatus | undefined = $state();
	let error: unknown = $state();
	let timer: ReturnType<typeof setTimeout> | undefined;
	let alive = true;

	async function refresh() {
		clearTimeout(timer);
		try {
			s = await serverStatus();
			error = undefined;
		} catch (e) {
			error = e;
		}
		if (alive) timer = setTimeout(() => void refresh(), POLL_MS);
	}
	void refresh();
	onDestroy(() => {
		alive = false;
		clearTimeout(timer);
	});

	const pct = (v: number) => `${Math.round(v)}%`;
	const since = (iso: string) => duration((Date.now() - Date.parse(iso)) / 1000);
	const cacheUse = (d: ServerStatus['disk']) =>
		d.transcode_limit > 0 ? Math.min(100, (100 * d.transcode) / d.transcode_limit) : 0;
</script>

<h1 class="mb-2 mt-8 text-[2.5rem] font-700">Status do servidor</h1>

{#if error !== undefined && s === undefined}
	<ErrorNotice {error} onretry={() => void refresh()} />
{:else if s !== undefined}
	<p class="meta mb-8 mt-0 text-text-secondary">
		Versão {s.version} · ligado há {duration(s.uptime_s)}
		{#if error !== undefined}· <span class="text-accent">sem resposta, tentando de novo…</span>{/if}
	</p>

	<div class="cards">
		<section class="card">
			<h2>Processador</h2>
			<p class="big">{pct(s.cpu.process)}</p>
			<p>do computador usado pelo servidor</p>
			{#if s.cpu.system !== null}<p>Computador todo: {pct(s.cpu.system)}</p>{/if}
		</section>

		<section class="card">
			<h2>Memória</h2>
			<p class="big">{bytes(s.memory.bytes)}</p>
			<p>usada pelo servidor (conversões à parte)</p>
		</section>

		<section class="card">
			<h2>Aceleração de vídeo</h2>
			<p class="big small">{encoderName(s.encoder)}</p>
			{#each s.gpus ?? [] as g (g.name)}
				<p>{g.name}: GPU {pct(g.utilization)} · codificador {pct(g.encoder)}</p>
			{/each}
		</section>

		<section class="card wide">
			<h2>Conversões ({s.slots.used} de {s.slots.limit} vagas)</h2>
			{#if s.sessions.length === 0}
				<p>Nenhuma agora. Quem toca direto não aparece aqui.</p>
			{:else}
				<ul>
					{#each s.sessions as x (x.started + x.media_id)}
						<li>
							<span class="file">{x.file || x.media_id}</span>
							<span class="detail">
								{x.encode ? 'vídeo convertido' : 'só o contêiner'}{x.audio === 'aac'
									? ' · áudio convertido'
									: ''}{x.burned ? ' · legenda gravada' : ''} · há {since(x.started)}
								{#if x.last_request !== undefined}· último pedido há {since(x.last_request)}{/if}
							</span>
						</li>
					{/each}
				</ul>
			{/if}
		</section>

		<section class="card">
			<h2>Disco</h2>
			<p>Cache de conversões: {bytes(s.disk.transcode)} de {bytes(s.disk.transcode_limit)}</p>
			<div class="bar"><div style:transform={`scaleX(${cacheUse(s.disk) / 100})`}></div></div>
			<p>Imagens: {bytes(s.disk.images)}</p>
			<p>Legendas: {bytes(s.disk.subtitles)}</p>
			<p>Banco de dados: {bytes(s.disk.database)}</p>
		</section>

		<section class="card">
			<h2>Biblioteca</h2>
			<p>{s.library.movies} filmes · {s.library.series} séries · {s.library.animes} animes</p>
			<p>{s.library.files} arquivos</p>
			{#if s.library.review > 0}<p>{s.library.review} para revisar</p>{/if}
		</section>
	</div>
{/if}

<style>
	.cards {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		grid-gap: 1.5rem;
	}
	.card {
		padding: 1.5rem 1.75rem;
		border-radius: 12px;
		background: #15151b;
	}
	.wide {
		grid-column: 1 / -1;
	}
	h2 {
		margin: 0 0 0.75rem;
		color: rgba(255, 255, 255, 0.6);
		font-size: 1.125rem;
		font-weight: 600;
	}
	p {
		margin: 0.25rem 0;
		font-size: 1.125rem;
	}
	.big {
		margin: 0 0 0.5rem;
		font-size: 2.5rem;
		font-weight: 700;
	}
	.big.small {
		font-size: 1.75rem;
	}
	ul {
		margin: 0;
		padding: 0;
		list-style: none;
	}
	li {
		padding: 0.75rem 0;
		border-top: 1px solid rgba(255, 255, 255, 0.08);
	}
	li:first-child {
		border-top: 0;
	}
	.file {
		display: block;
		font-weight: 600;
	}
	.detail {
		color: rgba(255, 255, 255, 0.6);
	}
	.bar {
		height: 0.5rem;
		margin: 0.5rem 0 1rem;
		overflow: hidden;
		border-radius: 999px;
		background: rgba(255, 255, 255, 0.1);
	}
	.bar > div {
		height: 100%;
		background: #4c8dff;
		transform-origin: left;
	}
	@media (max-width: 1100px) {
		.cards {
			grid-template-columns: minmax(0, 1fr);
		}
	}
	/* TV: text of at least 24px at 1080p (AGENTS.md §7). */
	:global(html.tv) .meta,
	:global(html.tv) h2,
	:global(html.tv) p,
	:global(html.tv) li {
		font-size: 1.5rem;
	}
	:global(html.tv) .big {
		font-size: 2.5rem;
	}
	:global(html.tv) .big.small {
		font-size: 1.75rem;
	}
</style>
