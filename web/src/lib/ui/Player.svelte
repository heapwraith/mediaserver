<script lang="ts">
	import { onDestroy, onMount, untrack } from 'svelte';

	import { goto } from '$app/navigation';

	import {
		ApiError,
		assURL,
		getMedia,
		getProgress,
		listFonts,
		nextEpisode,
		saveProgress,
		startPlayback,
		stopPlayback,
		vttURL,
	} from '#lib/api/client.ts';
	import { UNSUPPORTED, describeError, type Problem } from '#lib/api/errors.ts';
	import { session } from '#lib/api/session.svelte.ts';
	import type { HomeItem, Marker, Media, Playback, Track } from '#lib/api/types.ts';
	import { goBack } from '#lib/platform/back.ts';
	import { CONTAINERS, capabilities } from '#lib/platform/codecs.ts';
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
	import { settings } from '#lib/platform/settings.svelte.ts';
	import { homeTitle, playHref } from '#lib/ui/cards.ts';
	import {
		creditsAt,
		nextCountdown,
		resumeAt,
		seekAction,
		shouldSave,
		skipIntro,
		MIN_SAVE_S,
	} from '#lib/ui/progress.ts';
	import { attach, type Source } from '#lib/ui/source.ts';
	import { attachASS, defaultSubtitle, subtitleCSS, type ASSRenderer } from '#lib/ui/subtitles.ts';
	import { clock } from '#lib/ui/time.ts';

	// Own player on the native <video> (ADR 004): Chromium 76 safe; hls.js
	// and JASSUB attach to this same element.
	//
	// The server decides direct play or HLS from this device's codecs (ADR
	// 006). An HLS session starts at `offset` seconds of the media: the real
	// time is offset + currentTime, and seeking past what is ready (or
	// switching the audio track) asks for a new session there.
	//
	// Remote: left/right ±10 s, Enter play/pause. Up goes to the top bar
	// (Voltar · Legendas · Áudio · Volume), where left/right pick and Enter
	// opens; Voltar closes the panel, then the bar, then the player.
	//
	// Progress: resumes where the profile stopped (unless fromStart) and saves
	// every 10 s, on pause and on leaving. In the last seconds a "Próximo
	// episódio" card takes left/right/Enter; Voltar dismisses it.

	const HIDE_MS = 3000;
	const SEEK_S = 10;
	const SEEK_SETTLE_MS = 600;
	const VOLUME_STEP = 0.1;
	const MAX_RETRIES = 2;

	type Tool = 'back' | 'subs' | 'audio' | 'volume' | 'fullscreen';
	type Panel = '' | 'subs' | 'audio' | 'volume';
	type Subtitle = Media['subtitles'][number];

	let {
		mediaId,
		title,
		fromStart = false,
	}: {
		mediaId: string;
		title: string;
		/** "Do início": ignore the saved position. */
		fromStart?: boolean;
	} = $props();

	let video: HTMLVideoElement | undefined = $state();
	let root: HTMLElement | undefined = $state();
	let fullscreen = $state(false);

	// A 16:9 video in a browser window (tabs, address bar) gets side bars;
	// the fullscreen button removes them. Leaving the player leaves fullscreen.
	function syncFullscreen() {
		fullscreen = isFullscreen();
	}
	// Leaving the page (tab closed, app sent to the background) saves the
	// position; closing it also stops the HLS session.
	const hide = () => {
		persist(true);
		stopSession();
	};
	function visibility() {
		if (document.visibilityState === 'hidden') persist(true);
	}
	onMount(() => {
		const sync = syncFullscreen;
		document.addEventListener('fullscreenchange', sync);
		document.addEventListener('webkitfullscreenchange', sync);
		window.addEventListener('pagehide', hide);
		document.addEventListener('visibilitychange', visibility);
		return () => {
			document.removeEventListener('fullscreenchange', sync);
			document.removeEventListener('webkitfullscreenchange', sync);
			window.removeEventListener('pagehide', hide);
			document.removeEventListener('visibilitychange', visibility);
			persist(true);
			clearTimeout(seekTimer);
			playToken += 1; // a play() still loading must not attach anymore
			source?.destroy();
			source = undefined;
			stopSession();
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
	let error: Problem | undefined = $state();
	let visible = $state(true);
	let hideTimer: ReturnType<typeof setTimeout> | undefined;

	let audio: Track[] = $state([]);
	let audioSel = $state(0);
	let subs: Subtitle[] = $state([]);
	let subSel = $state(-1); // -1 = off
	let appliedSub = $state(-1);
	let tracksReady = $state(false);
	let burned = $state(false);
	let burnRequested = false;
	let offset = $state(0);
	let assUnavailable = $state(false);
	const selectedSub = $derived(subs[appliedSub]);
	const canASS = $derived(!assUnavailable && capabilities().includes('ass'));
	const textURL = $derived(selectedSub?.kind === 'text' && !burned ? vttURL(selectedSub.id, offset) : '');
	let ass: ASSRenderer | undefined = $state.raw();
	let fontURLs: Promise<string[]> | undefined;

	$effect(() => {
		const sub = selectedSub;
		const el = video;
		if (sub?.kind !== 'ass' || el === undefined || !canASS || burned) return;
		let canceled = false;
		let renderer: ASSRenderer | undefined;
		fontURLs ??= listFonts(mediaId);
		void fontURLs
			.then((fonts) => canceled ? undefined : attachASS(el, assURL(sub.id), fonts, untrack(() => offset)))
			.then((r) => {
				if (canceled) r?.destroy();
				else if (r !== undefined) {
					renderer = r;
					r.setOffset(offset);
					ass = r;
				}
				return undefined;
			})
			.catch(() => {
				if (canceled) return;
				assUnavailable = true;
				void play(time);
			});
		return () => {
			canceled = true;
			renderer?.destroy();
			ass = undefined;
		};
	});
	$effect(() => {
		ass?.setOffset(offset);
	});

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

	async function loadTracks(id: string) {
		const m = await getMedia(id).catch(() => undefined);
		if (m === undefined) {
			tracksReady = true;
			return;
		}
		audio = m.audio;
		markers = m.markers ?? [];
		// The default track (or the first), like the server picks it.
		audioSel = Math.max(
			0,
			audio.findIndex((t) => t.default === true),
		);
		if (audioIndex < 0) audioIndex = audio[audioSel]?.index ?? -1;
		subs = m.subtitles.filter((s) => s.kind === 'text' || s.kind === 'ass');
		appliedSub = defaultSubtitle(subs, session.profile?.subtitle_language ?? '', audio[audioSel]?.language);
		subSel = appliedSub;
		tracksReady = true;
		if (duration === 0 && m.duration_ms !== undefined) duration = m.duration_ms / 1000;
	}

	// --- Playback: direct play or an HLS session (ADR 006) ---
	let playback: Playback | undefined;
	let source: Source | undefined;
	let audioIndex = -1; // ffprobe index of the audio track playing (-1: default)
	let pendingSeek: number | undefined; // direct play: seek once the metadata loads
	let retries = 0;
	let playToken = 0; // only the latest play() attaches
	let seekTimer: ReturnType<typeof setTimeout> | undefined; // a new session waiting for the last key press

	/** Stops the current HLS session on the server (frees its FFmpeg). */
	function stopSession() {
		if (playback !== undefined && !playback.direct) stopPlayback(playback).catch(() => undefined);
		playback = undefined;
	}

	/** Plays from `at` seconds with the audio track `track`; caps: what to declare. */
	async function play(
		at: number,
		track = audioIndex,
		caps = capabilities(),
		subId = selectedSub?.id ?? '',
	) {
		if (video === undefined) return;
		clearTimeout(seekTimer);
		seekTimer = undefined;
		if (assUnavailable) caps = caps.filter((c) => c !== 'ass');
		burnRequested = !caps.includes('ass') && subs.some((s) => s.id === subId && s.kind === 'ass');
		const token = ++playToken;
		const el = video;
		waiting = true;
		error = undefined;
		source?.destroy();
		source = undefined;
		// Free the old FFmpeg first: the server limits concurrent transcodes.
		const old = playback;
		playback = undefined;
		if (old !== undefined && !old.direct) await stopPlayback(old).catch(() => undefined);
		let p: Playback;
		try {
			p = await startPlayback(mediaId, caps, track, at, subId);
		} catch (e) {
			if (token === playToken) {
				waiting = false;
				// The server says why (file gone, busy, no FFmpeg…).
				error = describeError(e instanceof ApiError && e.code !== '' ? e : UNSUPPORTED);
			}
			return;
		}
		if (token !== playToken) {
			if (!p.direct) stopPlayback(p).catch(() => undefined);
			return;
		}
		playback = p;
		burned = p.burned === true;
		audioIndex = track;
		offset = p.direct ? 0 : p.start;
		pendingSeek = p.direct && p.start > 0 ? p.start : undefined;
		time = p.start;
		const s = await attach(el, p, onFatal);
		if (token === playToken) source = s;
		else s.destroy();
	}

	// Starts once the saved position is known.
	let started = false;
	$effect(() => {
		if (resume !== undefined && video !== undefined && tracksReady && !started) {
			started = true;
			const at = resume;
			untrack(() => void play(at));
		}
	});

	/** The HLS stream broke (session stopped, server restarted): resume here. */
	function onFatal() {
		if (retries >= MAX_RETRIES) {
			waiting = false;
			error = describeError(UNSUPPORTED);
			return;
		}
		retries += 1;
		void play(time);
	}

	/** The file itself failed: try once more through HLS (no container declared). */
	function onVideoError() {
		if (playback?.direct !== true) return; // HLS errors come through hls.js
		if (retries >= MAX_RETRIES) {
			error = describeError(UNSUPPORTED);
			return;
		}
		retries += 1;
		void play(
			time,
			audioIndex,
			capabilities().filter((c) => !CONTAINERS.includes(c)),
		);
	}
	$effect(() => {
		void loadTracks(mediaId);
	});

	// Saved position (seconds): the video loads once it is known.
	let resume: number | undefined = $state();
	let lastSave = 0;
	let next: HomeItem | undefined = $state();
	let nextCanceled = $state(false);
	let nextSel = $state(0); // 0 = Assistir agora, 1 = Cancelar
	// Opening and credits found by the server: "Pular abertura", and the
	// next-episode card as soon as the credits start.
	let markers: Marker[] = $state([]);
	const introEnd = $derived(skipIntro(time, markers));
	const countdown = $derived(
		next === undefined || nextCanceled ? null : nextCountdown(time, duration, creditsAt(markers)),
	);
	// With credits the countdown ends before the video does.
	$effect(() => {
		if (countdown === 0 && !paused && settings.autoplayNext && time < duration - 1) goNext();
	});

	function skipToEnd() {
		if (introEnd !== null) seekTo(introEnd);
	}

	$effect(() => {
		const id = mediaId;
		if (fromStart) resume = 0;
		else {
			getProgress(id).then(
				(p) => (resume = resumeAt(p)),
				() => (resume = 0),
			);
		}
		nextEpisode(id).then(
			(n) => (next = n),
			() => (next = undefined),
		);
	});

	/** Saves the position; keepalive lets it finish while the page closes. */
	function persist(keepalive = false) {
		if (resume === undefined || time < MIN_SAVE_S || !(duration > 0)) return;
		lastSave = Date.now();
		saveProgress(mediaId, time * 1000, duration * 1000, keepalive).catch(() => undefined);
	}

	function onTime() {
		// Between sessions, or before the first seek of a direct play, the
		// element's time is not the media's.
		// While a seek waits, `time` is where the user is going.
		if (video === undefined || source === undefined || pendingSeek !== undefined || seekTimer !== undefined) return;
		time = offset + video.currentTime;
		if (!paused && shouldSave(time, lastSave, Date.now())) persist();
	}

	let leaving = false; // the countdown effect and onEnded may both get here
	function goNext() {
		if (next === undefined || leaving) return;
		leaving = true;
		persist();
		// Replace: Voltar still goes back to the details page.
		void goto(playHref(next.media_id, homeTitle(next)), { replaceState: true });
	}

	function onEnded() {
		paused = true;
		persist();
		if (next !== undefined && !nextCanceled && settings.autoplayNext) goNext();
	}

	/** Keys while the "Próximo episódio" card shows. */
	function nextKey(a: Action): boolean {
		if (a === 'left' || a === 'right') nextSel = a === 'right' ? 1 : 0;
		else if (a === 'enter' && nextSel === 0) goNext();
		else if (a === 'enter' || a === 'back') nextCanceled = true;
		else return false;
		show();
		return true;
	}

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
		seekTo(time + delta, SEEK_SETTLE_MS);
	}

	/**
	 * Seeks to media second t: in place when it is ready, else a new HLS session
	 * there. settleMs: held arrow keys repeat and every session costs an FFmpeg,
	 * so it opens only after the last press.
	 */
	function seekTo(t: number, settleMs = 0) {
		show();
		if (video === undefined || !started) return;
		const end = duration > 0 ? duration : Infinity;
		const at = Math.min(Math.max(0, t), end);
		time = at;
		clearTimeout(seekTimer);
		seekTimer = undefined;
		if (
			playback !== undefined &&
			source !== undefined &&
			seekAction(at, offset, source.end(), playback.direct) === 'inplace'
		) {
			video.currentTime = playback.direct ? at : at - offset;
			return;
		}
		// Past what hls.js knows of the session the browser clamps the seek.
		playToken += 1; // a play() still loading would bring `time` back
		if (settleMs === 0) void play(at);
		else {
			seekTimer = setTimeout(() => {
				seekTimer = undefined;
				void play(time);
			}, settleMs);
		}
	}

	function open(p: Exclude<Panel, ''>) {
		if (p === 'subs') subSel = appliedSub;
		panel = panel === p ? '' : p;
		show();
	}

	function useTool(t: Tool) {
		if (t === 'back') goBack();
		else if (t === 'fullscreen') toggleFullscreen();
		else open(t);
	}

	/** Another audio track: the server remuxes/converts with it from here. */
	function chooseAudio(i: number) {
		audioSel = i;
		panel = '';
		const t = audio[i];
		if (t !== undefined && t.index !== audioIndex) void play(time, t.index);
		show();
	}

	function chooseSub(i: number) {
		const changed = appliedSub !== i;
		appliedSub = i;
		subSel = i;
		if (changed && (burned || burnRequested || (!canASS && subs[i]?.kind === 'ass'))) {
			void play(time, audioIndex, capabilities(), subs[i]?.id ?? '');
		}
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

	/** Remote keys with one meaning wherever the focus is (AGENTS.md §7, Sprint 7). */
	function remoteKey(a: Action): boolean {
		switch (a) {
			case 'play':
				if (video?.paused === true) toggle();
				else show();
				return true;
			case 'pause':
				if (video?.paused === false) toggle();
				else show();
				return true;
			case 'forward':
				seek(SEEK_S);
				return true;
			case 'rewind':
				seek(-SEEK_S);
				return true;
			case 'stop':
				goBack();
				return true;
			case 'red':
				if (subs.length > 0) open('subs');
				return true;
			case 'green':
				if (audio.length > 1) open('audio');
				return true;
			default:
				return false;
		}
	}

	function onKey(a: Action): boolean {
		if (remoteKey(a)) return true;
		if (panel !== '') return panelKey(a);
		if (inTop) return topKey(a);
		if (countdown !== null && nextKey(a)) return true;
		if (introEnd !== null && a === 'enter') {
			skipToEnd(); // OK skips the opening; play/pause stays on its own key
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

	function seekClick(e: MouseEvent) {
		const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
		if (duration > 0) seekTo(((e.clientX - r.left) / r.width) * duration);
	}

	function trackName(t: Track): string {
		const name = t.title ?? t.language?.toUpperCase() ?? `Faixa ${t.index}`;
		return [name, t.codec.toUpperCase(), t.channels ? `${t.channels}ch` : '']
			.filter(Boolean)
			.join(' · ');
	}

	const subName = (s: Subtitle) =>
		[s.title ?? s.language?.toUpperCase() ?? s.codec.toUpperCase(), s.forced ? 'Forçada' : '']
			.filter(Boolean)
			.join(' · ');

</script>

<svelte:head>
	<title>{title || 'Assistir'}</title>
	<!-- The values come from a fixed table (subtitleCSS). -->
	{@html `<style>video::cue{${subtitleCSS(settings.subtitleSize, settings.subtitleBackground)}}</style>`}
</svelte:head>

<div
	bind:this={root}
	class="player"
	class:idle={!visible}
	role="application"
	aria-label="Player"
	use:focusable={{ id: 'player', onKey }}
	onpointermove={show}
>
	<video
		bind:this={video}
		autoplay
		playsinline
		onplay={() => (paused = false)}
		onpause={() => {
			paused = true;
			persist();
		}}
		onended={onEnded}
		ontimeupdate={onTime}
		onloadedmetadata={() => {
			if (video !== undefined) {
				// An HLS session covers only part of the file: keep the API duration.
				if (playback?.direct === true && Number.isFinite(video.duration)) {
					duration = video.duration;
				}
				video.volume = volume;
				video.muted = muted;
				if (pendingSeek !== undefined) {
					video.currentTime = Math.min(pendingSeek, Math.max(0, (video.duration || pendingSeek) - 1));
					pendingSeek = undefined;
				}
			}
			show();
		}}
		onvolumechange={volumeChanged}
		onwaiting={() => (waiting = true)}
		onplaying={() => {
			waiting = false;
			retries = 0;
			show();
		}}
		oncanplay={() => (waiting = false)}
		onerror={onVideoError}
		onclick={onVideoClick}
	>
		{#key textURL}
			{#if textURL !== ''}
				<track
					kind="subtitles"
					default
					src={textURL}
					srclang={selectedSub?.language ?? 'und'}
					label={selectedSub ? subName(selectedSub) : 'Legendas'}
					onload={(e) => {
						if (e.currentTarget instanceof HTMLTrackElement) e.currentTarget.track.mode = 'showing';
					}}
				/>
			{/if}
		{/key}
	</video>

	{#if waiting && error === undefined}<div class="spinner" aria-label="Carregando"></div>{/if}
	{#if error !== undefined}
		<div class="error" role="alert">
			<p class="error-title">{error.title}</p>
			<p class="error-hint">{error.hint}</p>
		</div>
	{/if}

	{#if introEnd !== null && countdown === null}
		<button tabindex="-1" class="pill skip kfocus" onclick={skipToEnd}>Pular abertura ›</button>
	{/if}

	{#if countdown !== null && next !== undefined}
		{@const n = next}
		<div class="next" role="dialog" aria-label="Próximo episódio">
			<p class="next-label">Próximo episódio</p>
			<p class="next-title">{homeTitle(n)}{n.episode?.title ? ` · ${n.episode.title}` : ''}</p>
			{#if settings.autoplayNext}<p class="next-count">Começa em {countdown} s</p>{/if}
			<div class="next-buttons">
				<button tabindex="-1" class="pill" class:kfocus={nextSel === 0} onclick={goNext}>
					▶ Assistir agora
				</button>
				<button
					tabindex="-1"
					class="pill"
					class:kfocus={nextSel === 1}
					onclick={() => (nextCanceled = true)}
				>
					Cancelar
				</button>
			</div>
		</div>
	{/if}

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
				onclick={seekClick}
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
	/* "Pular abertura": bottom right, shown even with the controls hidden. */
	.skip {
		position: absolute;
		right: 5vw;
		bottom: 22vh;
		z-index: 1;
		height: 3.5rem;
		padding: 0 2rem;
		font-size: 1.25rem;
	}
	/* "Próximo episódio": bottom right, above the progress bar. */
	.next {
		position: absolute;
		right: 5vw;
		bottom: 22vh;
		z-index: 1;
		max-width: 32rem;
		padding: 1.5rem 1.75rem;
		border-radius: 12px;
		background: rgba(21, 21, 27, 0.94);
		box-shadow: 0 16px 40px rgba(0, 0, 0, 0.6);
	}
	.next-label {
		margin: 0;
		color: rgba(255, 255, 255, 0.6);
		font-size: 1rem;
	}
	.next-title {
		margin: 0.25rem 0 0;
		font-size: 1.5rem;
		font-weight: 600;
	}
	.next-count {
		margin: 0.5rem 0 0;
		color: rgba(255, 255, 255, 0.6);
	}
	.next-buttons {
		display: flex;
		margin-top: 1.25rem;
	}
	.next-buttons > :global(* + *) {
		margin-left: 1rem;
	}
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
	.error p {
		margin: 0;
	}
	.error-title {
		font-weight: 600;
	}
	.error-hint {
		margin-top: 0.75rem !important;
		color: rgba(255, 255, 255, 0.6);
		font-size: 1.25rem;
	}
	:global(html.tv) .error-hint {
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
	/* TV: text of at least 24px at 1080p (AGENTS.md §7). */
	:global(html.tv) .next-label,
	:global(html.tv) .pill,
	:global(html.tv) .time,
	:global(html.tv) .buttons button {
		font-size: 1.5rem;
	}
</style>
