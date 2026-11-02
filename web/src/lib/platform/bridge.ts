// What the TV shell (the small local page inside the .wgt/.ipk/.apk) and the
// app it shows (served by the server) tell each other (docs/adr/008).
// Tizen and webOS show the app in an <iframe> and talk through postMessage;
// Android navigates to the app and exposes `MediaserverAndroid` instead.

/** Where the app runs. `browser` is any PC browser (no shell). */
export type Platform = 'browser' | 'tizen' | 'webos' | 'android';

export const PLATFORMS: readonly Platform[] = ['browser', 'tizen', 'webos', 'android'];

export const isPlatform = (v: unknown): v is Platform =>
	typeof v === 'string' && (PLATFORMS as readonly string[]).includes(v);

/** Query parameter with which the shell opens the app (`/?tv=tizen`). */
export const PLATFORM_PARAM = 'tv';

/** App → shell. */
export interface AppMessage {
	from: 'mediaserver-app';
	/** ready: the app started · exit: leave the TV app · change-server: show the connection screen. */
	type: 'ready' | 'exit' | 'change-server';
}

/** Shell → app: a key the shell caught while the frame had no focus. */
export interface ShellMessage {
	from: 'mediaserver-shell';
	type: 'key';
	key: string;
	keyCode: number;
}

const appTypes = new Set(['ready', 'exit', 'change-server']);

export function appMessage(type: AppMessage['type']): AppMessage {
	return { from: 'mediaserver-app', type };
}

export function isAppMessage(data: unknown): data is AppMessage {
	return (
		typeof data === 'object' &&
		data !== null &&
		Reflect.get(data, 'from') === 'mediaserver-app' &&
		appTypes.has(String(Reflect.get(data, 'type')))
	);
}

export function keyMessage(key: string, keyCode: number): ShellMessage {
	return { from: 'mediaserver-shell', type: 'key', key, keyCode };
}

export function isShellMessage(data: unknown): data is ShellMessage {
	return (
		typeof data === 'object' &&
		data !== null &&
		Reflect.get(data, 'from') === 'mediaserver-shell' &&
		Reflect.get(data, 'type') === 'key' &&
		typeof Reflect.get(data, 'key') === 'string' &&
		typeof Reflect.get(data, 'keyCode') === 'number'
	);
}

/** The bridge the Android wrapper (tv/android) adds to every page it shows. */
export interface AndroidBridge {
	/** Closes the app. */
	exit(): void;
	/** Shows the connection screen again. */
	openSetup(): void;
	/** Opens the app of a server (http only). */
	connect(url: string): void;
	/** IPv4 address of the device on the local network ('' when offline). */
	localIp(): string;
	/** The screen modes as JSON (framerate.ts). Missing in an APK older than the server. */
	displayModes?(): string;
	/** Asks for a screen mode; 0 goes back to the default. */
	setDisplayMode?(id: number): void;
}

function isAndroidBridge(b: unknown): b is AndroidBridge {
	return (
		typeof b === 'object' &&
		b !== null &&
		['exit', 'openSetup', 'connect', 'localIp'].every(
			(m) => typeof Reflect.get(b, m) === 'function',
		)
	);
}

/** The Android bridge, when the page runs inside the wrapper. */
export function androidBridge(w: object): AndroidBridge | undefined {
	const b: unknown = Reflect.get(w, 'MediaserverAndroid');
	return isAndroidBridge(b) ? b : undefined;
}

/** What the TV shell (web/src/shell) asks the device itself (native.ts). */
export interface Native {
	platform: Platform;
	/** Enables the remote keys the TV only delivers on request. */
	registerKeys(): void;
	/** Closes the TV app. */
	exit(): void;
	/** IPv4 address of the device on the local network ('' when unknown). */
	localIp(): Promise<string>;
	/** Opens a server's app in place of the shell (Android); undefined: the shell frames it. */
	connect?: (url: string) => void;
}
