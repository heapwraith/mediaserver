<script lang="ts">
	import { onDestroy, onMount } from 'svelte';

	import { page } from '$app/state';

	import { getMedia, streamURL } from '#lib/api/client.ts';
	import type { Media, Track } from '#lib/api/types.ts';
	import { goBack } from '#lib/platform/back.ts';
	import {
		enterFullscreen,
		exitFullscreen,
		isFullscreen,
		isTV,
		load,
		save,
	} from '#lib/platform/device.ts';
	import type { Action } from '#lib/platform/keys.ts';
	import { focusable } from '#lib/platform/nav.svelte.ts';
	import { clock } from '#lib/ui/time.ts';

	// Own player on the native <video> (ADR 004): Chromium 76 safe; hls.js
	// (Sprint 5) and JASSUB (Sprint 6) attach to this same element.
	//
	// Remote: left/right ±10 s, Enter play/pause. Up goes to the top bar
	// (Voltar · Legendas · Áudio · Volume), where left/right pick and Enter
	// opens; Voltar closes the panel, then the bar, then the player.

	const HIDE_MS = 3000;
	const SEEK_S = 10;
	const VOLUME_STEP = 0.1;

	type AudioTrackList = { length: number; [n: number]: { enabled: boolean } | undefined };
	type Tool = 'back' | 'subs' | 'audio' | 'volume' | 'fullscreen';
	type Panel = '' | 'subs' | 'audio' | 'volume';
	type Subtitle = Media['subtitles'][number];

	const mediaId = $derived(page.params.mediaId ?? '');
	const title = $derived(page.url.searchParams.get('title') ?? '');

	let video: HTMLVideoElement | undefined = $state();
	let root: HTMLElement | undefined = $state();
	let fullscreen = $state(false);

	// A 16:9 video in a browser window (tabs, address bar) gets side bars;
	// the fullscreen button removes them. Leaving the player leaves fullscreen.
	function syncFullscreen() {
		fullscreen = isFullscreen();
	}
	onMount(() => {
		const sync = syncFullscreen;
		document.addEventListener('fullscreenchange', sync);
		document.addEventListener('webkitfullscreenchange', sync);
		return () => {
			document.removeEventListener('fullscreenchange', sync);
			document.removeEventListener('webkitfullscreenchange', sync);
			void exitFullscreen();
		};
	});

	function toggleFullscreen() {
		if (isFullscreen()) void exitFullscreen();
		else if (root !== undefined) void enterFullscreen(root);
		show();
	}
	// Read from the element's events only: a two-way bind would push the
	// initial `paused` back into the element and race with autoplay.
	let paused = $state(true);
	let time = $state(0);
	let duration = $state(0);
	let waiting = $state(true);
	let error = $state('');
	let visible = $state(true);
	let hideTimer: ReturnType<typeof setTimeout> | undefined;

	let audio: Track[] = $state([]);
	let audioSel = $state(0);
	let subs: Subtitle[] = $state([]);
	let subSel = $state(-1); // -1 = off

	let panel: Panel = $state('');
	let inTop = $state(false);
	let toolIdx = $state(1);
	const tools = $derived<Tool[]>([
		'back',
		...(subs.length > 0 ? (['subs'] as const) : []),
		...(audio.length > 1 ? (['audio'] as const) : []),
		'volume',
		...(isTV ? [] : (['fullscreen'] as const)),
	]);
	const kfocus = (t: Tool) => inTop && panel === '' && tools[toolIdx] === t;

	// Volume is remembered on the device.
	const VOLUME_KEY = 'mediaserver.volume';
	const savedVolume = load(VOLUME_KEY);
	let volume = $state(1);
	let muted = $state(false);
	if (typeof savedVolume === 'object' && savedVolume !== null) {
		if ('volume' in savedVolume && typeof savedVolume.volume === 'number') {
			volume = Math.min(1, Math.max(0, savedVolume.volume));
		}
		if ('muted' in savedVolume && typeof savedVolume.muted === 'boolean') muted = savedVolume.muted;
	}

	function setVolume(v: number) {
		if (video === undefined) return;
		video.volume = Math.round(Math.min(1, Math.max(0, v)) * 100) / 100;
		video.muted = video.volume === 0;
		show();
	}

	function toggleMute() {
		if (video === undefined) return;
		if (video.muted && video.volume === 0) video.volume = 0.5;
		video.muted = !video.muted;
		show();
	}

	function volumeChanged() {
		if (video === undefined) return;
		volume = video.volume;
		muted = video.muted;
		save(VOLUME_KEY, { volume, muted });
	}

	// Switching audio needs the AudioTrackList API (Safari); elsewhere it
	// comes with the transcoding (Sprint 5).
	const audioTracks = $derived(
		(video as (HTMLVideoElement & { audioTracks?: AudioTrackList }) | undefined)?.audioTracks,
	);

	async function loadTracks(id: string) {
		const m = await getMedia(id).catch(() => undefined);
		if (m === undefined) return;
		audio = m.audio;
		audioSel = Math.max(
			0,
			audio.findIndex((t) => t.default === true),
		);
		subs = m.subtitles;
		if (duration === 0 && m.duration_ms !== undefined) duration = m.duration_ms / 1000;
	}
	$effect(() => {
		void loadTracks(mediaId);
	});

	function show() {
		visible = true;
		clearTimeout(hideTimer);
		hideTimer = setTimeout(() => {
			if (!paused && panel === '' && !inTop) visible = false;
		}, HIDE_MS);
	}
	onDestroy(() => clearTimeout(hideTimer));

	function toggle() {
		if (video === undefined) return;
		if (video.paused) void video.play().catch(() => undefined);
		else video.pause();
		show();
	}

	function seek(delta: number) {
		if (video === undefined) return;
		const end = duration || video.duration || 0;
		video.currentTime = Math.min(Math.max(0, video.currentTime + delta), end);
		show();
	}

	function open(p: Exclude<Panel, ''>) {
		panel = panel === p ? '' : p;
		show();
	}

	function useTool(t: Tool) {
		if (t === 'back') goBack();
		else if (t === 'fullscreen') toggleFullscreen();
		else open(t);
	}

	function chooseAudio(i: number) {
		audioSel = i;
		if (audioTracks !== undefined) {
			for (let n = 0; n < audioTracks.length; n++) {
				const t = audioTracks[n];
				if (t !== undefined) t.enabled = n === i;
			}
		}
		panel = '';
		show();
	}

	function chooseSub(i: number) {
		subSel = i; // shown on screen from Sprint 6 (subtitle extraction)
		panel = '';
		show();
	}

	/** Keys while a panel is open. */
	function panelKey(a: Action): boolean {
		if (a === 'back') panel = '';
		else if (panel === 'volume') {
			if (a === 'up' || a === 'right') setVolume(volume + VOLUME_STEP);
			else if (a === 'down' || a === 'left') setVolume(volume - VOLUME_STEP);
			else if (a === 'enter') toggleMute();
		} else if (panel === 'audio') {
			if (a === 'up') audioSel = Math.max(0, audioSel - 1);
			else if (a === 'down') audioSel = Math.min(audio.length - 1, audioSel + 1);
			else if (a === 'enter') chooseAudio(audioSel);
		} else if (panel === 'subs') {
			if (a === 'up') subSel = Math.max(-1, subSel - 1);
			else if (a === 'down') subSel = Math.min(subs.length - 1, subSel + 1);
			else if (a === 'enter') chooseSub(subSel);
		}
		show();
		return true;
	}

	/** Keys in the top bar. */
	function topKey(a: Action): boolean {
		if (a === 'left') toolIdx = Math.max(0, toolIdx - 1);
		else if (a === 'right') toolIdx = Math.min(tools.length - 1, toolIdx + 1);
		else if (a === 'down' || a === 'back') inTop = false;
		else if (a === 'enter') useTool(tools[toolIdx] ?? 'back');
		else if (a === 'playpause') toggle();
		show();
		return true;
	}

	function onKey(a: Action): boolean {
		if (panel !== '') return panelKey(a);
		if (inTop) return topKey(a);
		switch (a) {
			case 'left':
				seek(-SEEK_S);
				return true;
			case 'right':
				seek(SEEK_S);
				return true;
			case 'enter':
			case 'playpause':
				toggle();
				return true;
			case 'up':
				inTop = true;
				toolIdx = Math.min(1, tools.length - 1); // first tool after Voltar
				show();
				return true;
			case 'down':
				show();
				return true;
			default:
				return false; // back: the layout leaves the player
		}
	}

	function onVideoClick() {
		if (panel !== '') {
			panel = '';
			show();
		} else toggle();
	}

	function seekTo(e: MouseEvent) {
		const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
		if (video !== undefined && duration > 0) {
			video.currentTime = ((e.clientX - r.left) / r.width) * duration;
		}
		show();
	}

	function trackName(t: Track): string {
		const name = t.title ?? t.language?.toUpperCase() ?? `Faixa ${t.index}`;
		return [name, t.codec.toUpperCase(), t.channels ? `${t.channels}ch` : '']
			.filter(Boolean)
			.join(' · ');
	}

	const subName = (s: Subtitle) => s.title ?? s.language?.toUpperCase() ?? s.codec.toUpperCase();

	const unsupported =
		'Este navegador não reproduz o formato deste arquivo. A conversão automática chega numa próxima versão.';
</script>

<svelte:head><title>{title || 'Assistir'}</title></svelte:head>

<div
	bind:this={root}
	class="player"
	class:idle={!visible}
	role="application"
	aria-label="Player"
	use:focusable={{ id: 'player', onKey }}
	onpointermove={show}
>
	<!-- Subtitles come in Sprint 6. -->
	<!-- svelte-ignore a11y_media_has_caption -->
	<video
		bind:this={video}
		src={streamURL(mediaId)}
		autoplay
		playsinline
		onplay={() => (paused = false)}
		onpause={() => (paused = true)}
		ontimeupdate={() => (time = video?.currentTime ?? 0)}
		onloadedmetadata={() => {
			if (video !== undefined) {
				if (Number.isFinite(video.duration)) duration = video.duration;
				video.volume = volume;
				video.muted = muted;
			}
			show();
		}}
		onvolumechange={volumeChanged}
		onwaiting={() => (waiting = true)}
		onplaying={() => {
			waiting = false;
			show();
		}}
		oncanplay={() => (waiting = false)}
		onerror={() => (error = unsupported)}
		onclick={onVideoClick}
	></video>

	{#if waiting && error === ''}<div class="spinner" aria-label="Carregando"></div>{/if}
	{#if error !== ''}<p class="error">{error}</p>{/if}

	<div class="overlay">
		<div class="top">
			<button tabindex="-1" class="pill" class:kfocus={kfocus('back')} onclick={() => goBack()}>
				‹ Voltar
			</button>
			<p class="title">{title}</p>
			<div class="tools">
				{#if subs.length > 0}
					<button
						tabindex="-1"
						class="pill icon"
						class:kfocus={kfocus('subs')}
						class:on={panel === 'subs'}
						onclick={() => open('subs')}
						aria-label="Legendas"
						title="Legendas"
					>
						<!-- subtitles: a screen with two caption lines -->
						<svg viewBox="0 0 24 24" width="24" height="24" aria-hidden="true">
							<rect x="3" y="5" width="18" height="14" rx="2" fill="none" stroke="currentColor" stroke-width="2" />
							<path d="M7 12h3M12 12h5M7 15.5h6M15 15.5h2" stroke="currentColor" stroke-width="2" />
						</svg>
					</button>
				{/if}
				{#if audio.length > 1}
					<button
						tabindex="-1"
						class="pill icon"
						class:kfocus={kfocus('audio')}
						class:on={panel === 'audio'}
						onclick={() => open('audio')}
						aria-label="Faixa de áudio"
						title="Faixa de áudio"
					>
						<!-- audio track: a speech bubble with sound waves -->
						<svg viewBox="0 0 24 24" width="24" height="24" aria-hidden="true">
							<path
								d="M4 5h16a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H10l-4 3v-3H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1z"
								fill="none"
								stroke="currentColor"
								stroke-width="2"
								stroke-linejoin="round"
							/>
							<path d="M8 10v2M11 8.5v5M14 9.5v3M17 10.5v1" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
						</svg>
					</button>
				{/if}
				<button
					tabindex="-1"
					class="pill icon"
					class:kfocus={kfocus('volume')}
					class:on={panel === 'volume'}
					onclick={() => open('volume')}
					aria-label="Volume"
					title="Volume"
				>
					<svg viewBox="0 0 24 24" width="24" height="24" aria-hidden="true">
						<path d="M4 9v6h4l5 4V5L8 9H4z" fill="currentColor" />
						{#if muted || volume === 0}
							<path d="M16 9l5 6M21 9l-5 6" stroke="currentColor" stroke-width="2" />
						{:else}
							<path d="M16 8.5a5 5 0 0 1 0 7" stroke="currentColor" stroke-width="2" fill="none" />
						{/if}
					</svg>
				</button>
				{#if !isTV}
					<button
						tabindex="-1"
						class="pill icon"
						class:kfocus={kfocus('fullscreen')}
						onclick={toggleFullscreen}
						aria-label={fullscreen ? 'Sair da tela cheia' : 'Tela cheia'}
						title={fullscreen ? 'Sair da tela cheia' : 'Tela cheia'}
					>
						<svg viewBox="0 0 24 24" width="24" height="24" aria-hidden="true">
							{#if fullscreen}
								<path
									d="M9 4v5H4M15 4v5h5M9 20v-5H4M15 20v-5h5"
									fill="none"
									stroke="currentColor"
									stroke-width="2"
								/>
							{:else}
								<path
									d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5"
									fill="none"
									stroke="currentColor"
									stroke-width="2"
								/>
							{/if}
						</svg>
					</button>
				{/if}
			</div>

			{#if panel === 'audio'}
				<div class="panel" role="listbox" aria-label="Áudio">
					<p class="panel-title">Áudio</p>
					{#each audio as t, i (t.index)}
						<!-- svelte-ignore a11y_click_events_have_key_events -->
						<div
							class="option"
							class:sel={i === audioSel}
							role="option"
							tabindex="-1"
							aria-selected={i === audioSel}
							onclick={() => chooseAudio(i)}
						>
							{trackName(t)}
						</div>
					{/each}
					{#if audioTracks === undefined}
						<p class="hint">
							Este navegador toca só a faixa padrão. A troca de áudio chega com a conversão
							automática.
						</p>
					{/if}
				</div>
			{:else if panel === 'subs'}
				<div class="panel" role="listbox" aria-label="Legendas">
					<p class="panel-title">Legendas</p>
					<!-- svelte-ignore a11y_click_events_have_key_events -->
					<div
						class="option"
						class:sel={subSel === -1}
						role="option"
						tabindex="-1"
						aria-selected={subSel === -1}
						onclick={() => chooseSub(-1)}
					>
						Desligadas
					</div>
					{#each subs as s, i (s.id)}
						<!-- svelte-ignore a11y_click_events_have_key_events -->
						<div
							class="option"
							class:sel={i === subSel}
							role="option"
							tabindex="-1"
							aria-selected={i === subSel}
							onclick={() => chooseSub(i)}
						>
							{subName(s)}
						</div>
					{/each}
					<p class="hint">A exibição das legendas chega numa próxima versão.</p>
				</div>
			{:else if panel === 'volume'}
				<div class="panel volume-panel" aria-label="Volume">
					<p class="panel-title">Volume · {muted ? 'mudo' : `${Math.round(volume * 100)}%`}</p>
					<div class="volume-row">
						<button tabindex="-1" class="pill" onclick={toggleMute}>
							{muted ? 'Ativar som' : 'Silenciar'}
						</button>
						<input
							type="range"
							class="volume"
							min="0"
							max="1"
							step="0.05"
							tabindex="-1"
							aria-label="Volume"
							value={muted ? 0 : volume}
							oninput={(e) => setVolume(Number(e.currentTarget.value))}
							onchange={(e) => e.currentTarget.blur()}
						/>
					</div>
				</div>
			{/if}
		</div>

		<div class="bottom">
			<!-- The remote seeks with left/right; the mouse clicks the bar. -->
			<!-- svelte-ignore a11y_click_events_have_key_events -->
			<div
				class="bar"
				role="slider"
				aria-label="Progresso"
				aria-valuenow={Math.round(time)}
				aria-valuemin={0}
				aria-valuemax={Math.round(duration)}
				tabindex="-1"
				onclick={seekTo}
			>
				<div
					class="fill"
					style:transform={`scaleX(${duration > 0 ? Math.min(1, time / duration) : 0})`}
				></div>
			</div>
			<div class="controls">
				<span class="time">{clock(time)}</span>
				<div class="buttons">
					<button tabindex="-1" onclick={() => seek(-SEEK_S)} aria-label="Voltar 10 segundos">
						−10
					</button>
					<button
						tabindex="-1"
						class="play"
						onclick={toggle}
						aria-label={paused ? 'Reproduzir' : 'Pausar'}
					>
						{paused ? '▶' : '❚❚'}
					</button>
					<button tabindex="-1" onclick={() => seek(SEEK_S)} aria-label="Avançar 10 segundos">
						+10
					</button>
				</div>
				<span class="time end">−{clock(duration - time)}</span>
			</div>
		</div>
	</div>
</div>

<style>
	.player {
		position: fixed;
		top: 0;
		left: 0;
		width: 100vw;
		height: 100vh;
		background: #000;
		outline: none;
	}
	.player.idle {
		cursor: none;
	}
	video {
		width: 100%;
		height: 100%;
		object-fit: contain;
		background: #000;
	}
	.overlay {
		position: absolute;
		top: 0;
		left: 0;
		width: 100%;
		height: 100%;
		pointer-events: none;
		background:
			linear-gradient(to top, rgba(0, 0, 0, 0.75) 0%, rgba(0, 0, 0, 0) 35%),
			linear-gradient(to bottom, rgba(0, 0, 0, 0.6) 0%, rgba(0, 0, 0, 0) 20%);
		transition: opacity 250ms ease-out;
	}
	.idle .overlay {
		opacity: 0;
	}
	.top,
	.bottom {
		pointer-events: auto;
	}

	/* Top bar: Voltar + title on the left, tools on the right. */
	.top {
		position: absolute;
		top: 5vh;
		left: 5vw;
		right: 5vw;
		display: flex;
		align-items: center;
	}
	.title {
		flex: 1;
		margin: 0 0 0 1.5rem;
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
		font-size: 1.75rem;
		font-weight: 600;
	}
	.tools {
		display: flex;
		flex: none;
	}
	.tools > :global(* + *) {
		margin-left: 0.75rem;
	}
	.pill {
		display: flex;
		align-items: center;
		justify-content: center;
		height: 3rem;
		padding: 0 1.5rem;
		border: 0;
		border-radius: 999px;
		background: rgba(255, 255, 255, 0.14);
		color: rgba(255, 255, 255, 0.92);
		font: inherit;
		font-size: 1.125rem;
		font-weight: 600;
		cursor: pointer;
		transition: transform 200ms ease-out;
	}
	.pill.icon {
		width: 3rem;
		padding: 0;
	}
	.pill.on {
		background: rgba(255, 255, 255, 0.92);
		color: #0b0b0f;
	}
	/* Focus with the remote (AGENTS.md §7: scale + outline). */
	.pill.kfocus {
		transform: scale(1.08);
		box-shadow:
			0 12px 30px rgba(0, 0, 0, 0.5),
			0 0 0 3px #4c8dff;
	}
	.panel {
		position: absolute;
		top: 4.5rem;
		right: 0;
		width: 28rem;
		padding: 1rem;
		border-radius: 12px;
		background: rgba(30, 30, 38, 0.96);
	}
	.panel-title {
		margin: 0 0 0.5rem;
		font-weight: 600;
	}
	.option {
		padding: 0.75rem 1rem;
		border-radius: 8px;
		cursor: pointer;
		color: rgba(255, 255, 255, 0.8);
	}
	.option.sel {
		background: rgba(255, 255, 255, 0.92);
		color: #0b0b0f;
	}
	.hint {
		margin: 0.75rem 0 0;
		font-size: 0.875rem;
		color: rgba(255, 255, 255, 0.6);
	}
	.volume-row {
		display: flex;
		align-items: center;
	}
	.volume {
		flex: 1;
		margin-left: 1rem;
		cursor: pointer;
	}

	/* Bottom: progress bar and the play controls, centered on screen. */
	.bottom {
		position: absolute;
		left: 5vw;
		right: 5vw;
		bottom: 5vh;
	}
	.bar {
		height: 0.75rem;
		border-radius: 999px;
		background: rgba(255, 255, 255, 0.25);
		overflow: hidden;
		cursor: pointer;
	}
	.fill {
		height: 100%;
		background: #4c8dff;
		transform-origin: left;
	}
	.controls {
		display: grid;
		grid-template-columns: 1fr auto 1fr; /* equal sides keep the buttons centered */
		align-items: center;
		margin-top: 1rem;
	}
	.time {
		font-size: 1.125rem;
		font-variant-numeric: tabular-nums;
		color: rgba(255, 255, 255, 0.8);
	}
	.time.end {
		text-align: right;
	}
	.buttons {
		display: flex;
		align-items: center;
	}
	.buttons button {
		margin: 0 0.5rem;
		min-width: 4rem;
		height: 3rem;
		border: 0;
		border-radius: 999px;
		background: rgba(255, 255, 255, 0.14);
		color: rgba(255, 255, 255, 0.92);
		font: inherit;
		font-size: 1.125rem;
		font-weight: 600;
		cursor: pointer;
	}
	.buttons .play {
		min-width: 5rem;
		background: rgba(255, 255, 255, 0.92);
		color: #0b0b0f;
	}
	.error {
		position: absolute;
		top: 45%;
		left: 10vw;
		right: 10vw;
		margin: 0;
		text-align: center;
		font-size: 1.5rem;
	}
	.spinner {
		position: absolute;
		top: 50%;
		left: 50%;
		width: 4rem;
		height: 4rem;
		margin: -2rem 0 0 -2rem;
		border: 0.375rem solid rgba(255, 255, 255, 0.2);
		border-top-color: rgba(255, 255, 255, 0.92);
		border-radius: 50%;
		animation: spin 900ms linear infinite;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
</style>
