// Which platform the app runs on, and what only a TV can do (leave the app,
// pick another server). The rest of the app asks here instead of testing the
// platform itself (AGENTS.md §5.2).

import {
	PLATFORM_PARAM,
	androidBridge,
	appMessage,
	isShellMessage,
	type AppMessage,
	type Platform,
} from './bridge';
import { bestMode, parseDisplayModes, type DisplayModes } from './framerate';
import { dispatchKey } from './keys';

export type { Platform } from './bridge';

const STORE = 'mediaserver.tv';

interface Env {
	/** location.search of the app. */
	search: string;
	/** Platform saved in this tab (sessionStorage) by an earlier load. */
	stored: string | null;
	/** The app is inside a frame (the Tizen/webOS shell). */
	framed: boolean;
	/** The Android wrapper's bridge is there. */
	android: boolean;
}

/**
 * The shell opens the app with `?tv=<platform>`; the app keeps it for the
 * tab, since the parameter is gone after the first navigation.
 */
export function detectPlatform(env: Env): Platform {
	if (env.android) return 'android';
	const param = new URLSearchParams(env.search).get(PLATFORM_PARAM);
	const p = param ?? env.stored;
	// Tizen and webOS show the app in the shell's frame: with no frame there
	// is nobody to talk to, so it is a browser tab.
	if ((p === 'tizen' || p === 'webos') && env.framed) return p;
	return 'browser';
}

function current(): Platform {
	if (typeof window === 'undefined') return 'browser';
	let stored: string | null = null;
	try {
		stored = sessionStorage.getItem(STORE);
	} catch {
		// Storage blocked: the query parameter still works for this load.
	}
	const p = detectPlatform({
		search: location.search,
		stored,
		framed: window.parent !== window,
		android: androidBridge(window) !== undefined,
	});
	try {
		if (p === 'browser') sessionStorage.removeItem(STORE);
		else sessionStorage.setItem(STORE, p);
	} catch {
		// Not remembered: a reload inside the TV app falls back to the browser look.
	}
	return p;
}

export const platform: Platform = current();

/** Whether the app runs as a TV app. */
export const isTV = platform !== 'browser';

function toShell(type: AppMessage['type']): void {
	// The shell is a file:// page (origin "null"): it checks the source instead.
	window.parent.postMessage(appMessage(type), '*');
}

/** Leaves the app (TV only; the browser has nothing to exit). */
export function exitApp(): void {
	if (platform === 'android') androidBridge(window)?.exit();
	else if (platform === 'tizen' || platform === 'webos') toShell('exit');
}

/** Goes back to the TV app's connection screen (another server). */
export function changeServer(): void {
	if (platform === 'android') androidBridge(window)?.openSetup();
	else if (platform === 'tizen' || platform === 'webos') toShell('change-server');
}

function displayModes(): DisplayModes | undefined {
	if (platform !== 'android') return undefined;
	const raw = androidBridge(window)?.displayModes?.();
	return raw === undefined ? undefined : parseDisplayModes(raw);
}

/** Refresh rates of this screen at its current resolution, lowest first (Android only). */
export function displayRates(): number[] {
	const d = displayModes();
	const now = d?.modes.find((m) => m.id === d.current);
	if (d === undefined || now === undefined) return [];
	const same = d.modes.filter((m) => m.width === now.width && m.height === now.height);
	return [...new Set(same.map((m) => Math.round(m.rate * 100) / 100))].sort((a, b) => a - b);
}

/** Asks for the screen mode that suits a video; 0 goes back to the default. */
export function matchFrameRate(fps: number): void {
	const d = displayModes();
	if (d === undefined) return;
	androidBridge(window)?.setDisplayMode?.(fps > 0 ? bestMode(d, fps) : 0);
}

/**
 * Tells the shell the app started (it shows the connection screen again when
 * this never comes) and takes the keys it forwards.
 */
export function connectShell(): void {
	if (platform !== 'tizen' && platform !== 'webos') return;
	window.addEventListener('message', (e) => {
		if (e.source === window.parent && isShellMessage(e.data))
			dispatchKey(e.data.key, e.data.keyCode);
	});
	toShell('ready');
}

/**
 * Formats the TV plays natively although the browser checks deny them:
 * Samsung and LG answer '' for Matroska and Dolby audio but their players
 * handle both (the same table Jellyfin uses). A direct play that still fails
 * retries without the containers (lib/ui/Player.svelte).
 */
export function extraCapabilities(p: Platform = platform): string[] {
	return p === 'tizen' || p === 'webos' ? ['mkv', 'ac3', 'eac3'] : [];
}
