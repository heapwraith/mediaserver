<script lang="ts">
	import { onDestroy } from 'svelte';

	import { goto } from '$app/navigation';

	import { fetchHealth } from '#lib';
	import { downloadBackup, scanStatus, startScan } from '#lib/api/client.ts';
	import { session } from '#lib/api/session.svelte.ts';
	import type { ScanStatus } from '#lib/api/types.ts';
	import { capabilities } from '#lib/platform/codecs.ts';
	import { focusGroup } from '#lib/platform/nav.svelte.ts';
	import { changeServer, isTV, platform } from '#lib/platform/platform.ts';
	import { saveSettings, settings } from '#lib/platform/settings.svelte.ts';
	import Avatar from '#lib/ui/Avatar.svelte';
	import Button from '#lib/ui/Button.svelte';
	import ErrorNotice from '#lib/ui/ErrorNotice.svelte';

	const POLL_MS = 1000;

	let status: ScanStatus | undefined = $state();
	let error: unknown = $state();
	let timer: ReturnType<typeof setTimeout> | undefined;
	const health = fetchHealth().catch(() => undefined);

	async function refresh() {
		clearTimeout(timer);
		try {
			status = await scanStatus();
			error = undefined;
		} catch (e) {
			error = e;
		}
		if (status?.running === true) timer = setTimeout(() => void refresh(), POLL_MS);
	}
	void refresh();
	onDestroy(() => clearTimeout(timer));

	async function scan() {
		await startScan().catch((e: unknown) => (error = e));
		await refresh();
	}

	function toggle(key: 'reduceMotion' | 'backgroundTrailer' | 'autoplayNext') {
		settings[key] = !settings[key];
		saveSettings();
	}

	// "Este aparelho": what the server is told this device plays, to check
	// direct play on each TV (Sprint 7).
	const platformName = {
		browser: 'Navegador',
		tizen: 'Samsung (Tizen)',
		webos: 'LG (webOS)',
		android: 'Android TV',
	}[platform];
	const caps = capabilities();

	// Backup of profiles, progress and Revisar corrections; restored from the
	// command line with the server stopped (README: Backup e restauração).
	let backupNote = $state('');
	let backupError: unknown = $state();
	async function backup() {
		backupError = undefined;
		backupNote = 'Preparando o backup…';
		try {
			const name = await downloadBackup();
			backupNote = `Backup salvo: ${name}. Para restaurar, pare o servidor e rode: mediaserver restore ${name}`;
		} catch (e) {
			backupNote = '';
			backupError = e;
		}
	}

	const when = (iso?: string) => (iso === undefined ? '—' : new Date(iso).toLocaleString('pt-BR'));
</script>

<h1>Configurações</h1>

{#if session.profile}
	{@const p = session.profile}
	<section>
		<h2>Perfil</h2>
		<div class="profile">
			<Avatar name={p.name} color={p.color} size={4} />
			<span class="name">{p.name}</span>
		</div>
		<div class="bar" use:focusGroup={{ orientation: 'horizontal' }}>
			<Button id="settings:profile" onclick={() => goto(`/perfis/${p.id}`)}>Editar perfil</Button>
			<Button onclick={() => goto('/perfis')}>Trocar perfil</Button>
		</div>
	</section>
{/if}

<section>
	<h2>Biblioteca</h2>
	<div class="bar" use:focusGroup={{ orientation: 'horizontal' }}>
		<Button primary id="settings:scan" onclick={scan}>
			{status?.running === true ? 'Escaneando…' : 'Escanear agora'}
		</Button>
	</div>
	{#if error !== undefined}<ErrorNotice {error} onretry={() => void refresh()} />{/if}
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
	<h2>Reprodução</h2>
	<div class="bar" use:focusGroup={{ orientation: 'horizontal' }}>
		<Button selected={settings.autoplayNext} onclick={() => toggle('autoplayNext')}>
			Próximo episódio automático: {settings.autoplayNext ? 'sim' : 'não'}
		</Button>
	</div>
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

<section>
	<h2>Servidor</h2>
	<div class="bar" use:focusGroup={{ orientation: 'horizontal' }}>
		<Button id="settings:status" onclick={() => goto('/status')}>Status do servidor</Button>
		{#if !isTV}
			<!-- A TV has nowhere to save the file. -->
			<Button onclick={backup}>Baixar backup</Button>
		{/if}
	</div>
	{#if backupNote !== ''}<p class="note">{backupNote}</p>{/if}
	{#if backupError !== undefined}<ErrorNotice error={backupError} onretry={() => void backup()} />{/if}
</section>

<section>
	<h2>Este aparelho</h2>
	{#if isTV}
		<div class="bar" use:focusGroup={{ orientation: 'horizontal' }}>
			<Button onclick={changeServer}>Trocar servidor</Button>
		</div>
	{/if}
	<dl>
		<dt>Plataforma</dt>
		<dd>{platformName}</dd>
		<dt>Reproduz direto</dt>
		<dd>{caps.length > 0 ? caps.join(' · ') : '—'}</dd>
		<dt>Legendas ASS no aparelho</dt>
		<dd>{caps.includes('ass') ? 'sim' : 'não (gravadas no vídeo pelo servidor)'}</dd>
	</dl>
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
	.profile {
		display: flex;
		align-items: center;
		margin-bottom: 1.5rem;
	}
	.name {
		margin-left: 1.25rem;
		font-size: 1.5rem;
		font-weight: 600;
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
	/* TV: text of at least 24px at 1080p (AGENTS.md §7). */
	:global(html.tv) dl {
		font-size: 1.5rem;
	}
</style>
