<script lang="ts">
	import { onDestroy } from 'svelte';

	import { fetchHealth } from '#lib';
	import { scanStatus, startScan } from '#lib/api/client.ts';
	import type { ScanStatus } from '#lib/api/types.ts';
	import { focusGroup } from '#lib/platform/nav.svelte.ts';
	import { saveSettings, settings } from '#lib/platform/settings.svelte.ts';
	import Button from '#lib/ui/Button.svelte';

	const POLL_MS = 1000;

	let status: ScanStatus | undefined = $state();
	let error = $state('');
	let timer: ReturnType<typeof setTimeout> | undefined;
	const health = fetchHealth().catch(() => undefined);

	async function refresh() {
		clearTimeout(timer);
		try {
			status = await scanStatus();
			error = '';
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		}
		if (status?.running === true) timer = setTimeout(() => void refresh(), POLL_MS);
	}
	void refresh();
	onDestroy(() => clearTimeout(timer));

	async function scan() {
		await startScan().catch((e: unknown) => (error = e instanceof Error ? e.message : String(e)));
		await refresh();
	}

	function toggle(key: 'reduceMotion' | 'backgroundTrailer') {
		settings[key] = !settings[key];
		saveSettings();
	}

	const when = (iso?: string) => (iso === undefined ? '—' : new Date(iso).toLocaleString('pt-BR'));
</script>

<h1>Configurações</h1>

<section>
	<h2>Biblioteca</h2>
	<div class="bar" use:focusGroup={{ orientation: 'horizontal' }}>
		<Button primary id="settings:scan" onclick={scan}>
			{status?.running === true ? 'Escaneando…' : 'Escanear agora'}
		</Button>
	</div>
	{#if error}<p class="note">Erro: {error}</p>{/if}
	{#if status}
		<dl>
			<dt>Último scan</dt>
			<dd>{status.running ? 'em andamento' : when(status.finished_at)}</dd>
			<dt>Arquivos vistos</dt>
			<dd>{status.seen}</dd>
			<dt>Novos / alterados / removidos</dt>
			<dd>{status.added} / {status.updated} / {status.removed}</dd>
			<dt>Para revisar</dt>
			<dd>{status.review}</dd>
			<dt>Ainda copiando</dt>
			<dd>{status.pending}</dd>
			{#if status.probe_errors > 0}
				<dt>Erros de leitura (ffprobe)</dt>
				<dd>{status.probe_errors}</dd>
			{/if}
			{#if status.last_error}
				<dt>Último erro</dt>
				<dd>{status.last_error}</dd>
			{/if}
		</dl>
	{/if}
</section>

<section>
	<h2>Aparência</h2>
	<div class="bar" use:focusGroup={{ orientation: 'horizontal' }}>
		<Button selected={settings.reduceMotion} onclick={() => toggle('reduceMotion')}>
			Reduzir animações: {settings.reduceMotion ? 'sim' : 'não'}
		</Button>
		<Button selected={settings.backgroundTrailer} onclick={() => toggle('backgroundTrailer')}>
			Trailer no fundo: {settings.backgroundTrailer ? 'sim' : 'não'}
		</Button>
	</div>
</section>

{#await health then h}
	{#if h}<p class="note">Servidor {h.version} · banco {h.db}</p>{/if}
{/await}

<style>
	h1 {
		margin: 2rem 0 1rem;
		font-size: 2.5rem;
	}
	h2 {
		margin: 2rem 0 1rem;
		font-size: 1.5rem;
	}
	.bar {
		display: flex;
	}
	.bar > :global(* + *) {
		margin-left: 1rem;
	}
	dl {
		display: grid;
		grid-template-columns: max-content auto;
		grid-gap: 0.5rem 2rem;
		margin: 1.5rem 0 0;
		font-size: 1.125rem;
	}
	dt {
		color: rgba(255, 255, 255, 0.6);
	}
	dd {
		margin: 0;
	}
	.note {
		margin-top: 2rem;
		color: rgba(255, 255, 255, 0.6);
	}
</style>
