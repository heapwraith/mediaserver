<script lang="ts">
	import { onDestroy } from 'svelte';

	import { page } from '$app/state';

	import { getMedia, streamURL } from '#lib/api/client.ts';
	import type { Track } from '#lib/api/types.ts';
	import type { Action } from '#lib/platform/keys.ts';
	import { focusable } from '#lib/platform/nav.svelte.ts';
	import { clock } from '#lib/ui/time.ts';

	// Own player on the native <video> (ADR 004): Chromium 76 safe; hls.js
	// (Sprint 5) and JASSUB (Sprint 6) attach to this same element.

	const HIDE_MS = 3000;
	const SEEK_S = 10;

	type AudioTrackList = { length: number; [n: number]: { enabled: boolean } | undefined };

	const mediaId = $derived(page.params.mediaId ?? '');
	const title = $derived(page.url.searchParams.get('title') ?? '');

	let video: HTMLVideoElement | undefined = $state();
	// Read from the element's events only: a two-way bind would push the
	// initial `paused` back into the element and race with autoplay.
	let paused = $state(true);
	let time = $state(0);
	let duration = $state(0);
	let waiting = $state(true);
	let error = $state('');
	let visible = $state(true);
	let menu = $state(false);
	let audio: Track[] = $state([]);
	let audioSel = $state(0);
	let hideTimer: ReturnType<typeof setTimeout> | undefined;

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
		if (duration === 0 && m.duration_ms !== undefined) duration = m.duration_ms / 1000;
	}
	$effect(() => {
		void loadTracks(mediaId);
	});

	function show() {
		visible = true;
		clearTimeout(hideTimer);
		hideTimer = setTimeout(() => {
			if (!paused && !menu) visible = false;
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

	function chooseAudio(i: number) {
		audioSel = i;
		if (audioTracks !== undefined) {
			for (let n = 0; n < audioTracks.length; n++) {
				const t = audioTracks[n];
				if (t !== undefined) t.enabled = n === i;
			}
		}
		menu = false;
		show();
	}

	function onKey(a: Action): boolean {
		if (menu) {
			if (a === 'up') audioSel = Math.max(0, audioSel - 1);
			else if (a === 'down') audioSel = Math.min(audio.length - 1, audioSel + 1);
			else if (a === 'enter' && audioTracks !== undefined) chooseAudio(audioSel);
			else if (a === 'back' || a === 'enter') menu = false;
			show();
			return true;
		}
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
			case 'down':
				if (audio.length > 1) menu = true;
				show();
				return true;
			case 'up':
				show();
				return true;
			default:
				return false; // back: the layout leaves the player
		}
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

	const unsupported =
		'Este navegador não reproduz o formato deste arquivo. A conversão automática chega numa próxima versão.';
</script>

<svelte:head><title>{title || 'Assistir'}</title></svelte:head>

<div
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
			if (video !== undefined && Number.isFinite(video.duration)) duration = video.duration;
			show();
		}}
		onwaiting={() => (waiting = true)}
		onplaying={() => {
			waiting = false;
			show();
		}}
		oncanplay={() => (waiting = false)}
		onerror={() => (error = unsupported)}
		onclick={toggle}
	></video>

	{#if waiting && error === ''}<div class="spinner" aria-label="Carregando"></div>{/if}
	{#if error !== ''}<p class="error">{error}</p>{/if}

	<div class="overlay">
		<p class="title">{title}</p>
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
					{#if audio.length > 1}
						<button tabindex="-1" onclick={() => (menu = !menu)}>Áudio</button>
					{/if}
				</div>
				<span class="time end">−{clock(duration - time)}</span>
			</div>
		</div>
		{#if menu}
			<div class="menu" role="listbox" aria-label="Áudio">
				<p class="menu-title">Áudio</p>
				{#each audio as t, i (t.index)}
					<!-- svelte-ignore a11y_click_events_have_key_events -->
					<div
						class="option"
						class:sel={i === audioSel}
						role="option"
						tabindex="-1"
						aria-selected={i === audioSel}
						aria-disabled={audioTracks === undefined}
						onclick={() => (audioTracks === undefined ? (audioSel = i) : chooseAudio(i))}
					>
						{trackName(t)}
					</div>
				{/each}
				{#if audioTracks === undefined}
					<p class="hint">
						Este navegador toca só a faixa padrão. A troca de áudio chega com a conversão automática.
					</p>
				{/if}
			</div>
		{/if}
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
	.title,
	.bottom,
	.menu {
		pointer-events: auto;
	}
	.title {
		position: absolute;
		top: 5vh;
		left: 5vw;
		margin: 0;
		font-size: 1.75rem;
		font-weight: 600;
	}
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
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-top: 1rem;
	}
	.time {
		min-width: 6rem;
		font-size: 1.125rem;
		font-variant-numeric: tabular-nums;
		color: rgba(255, 255, 255, 0.8);
	}
	.time.end {
		text-align: right;
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
	.menu {
		position: absolute;
		right: 5vw;
		bottom: 18vh;
		width: 28rem;
		padding: 1rem;
		border-radius: 12px;
		background: rgba(30, 30, 38, 0.96);
	}
	.menu-title {
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
