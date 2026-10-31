// The server the TV app connects to: what the user typed, where it is saved
// on the device and the address of its app.

import { PLATFORM_PARAM, type Platform } from '../lib/platform/bridge';
import { load, save } from '../lib/platform/device';

export interface Server {
	host: string;
	port: number;
}

export const DEFAULT_PORT = 8080;

const KEY = 'mediaserver.server';

/**
 * Reads the address fields. The host field also takes "ip:port" and a
 * pasted URL. Returns the server or the message to show.
 */
export function parseServer(hostInput: string, portInput: string): Server | string {
	let host = hostInput
		.trim()
		.replace(/^https?:\/\//i, '')
		.replace(/\/$/, '');
	let port = portInput.trim();
	const withPort = /^(.+):(\d+)$/.exec(host);
	if (withPort !== null) {
		host = withPort[1] ?? '';
		port = withPort[2] ?? '';
	}
	if (host === '') return 'Digite o endereço do servidor.';
	if (!/^[a-z0-9.-]+$/i.test(host)) return 'Endereço inválido.';
	const n = port === '' ? DEFAULT_PORT : Number(port);
	if (!Number.isInteger(n) || n < 1 || n > 65535) return 'Porta inválida.';
	return { host: host.toLowerCase(), port: n };
}

/** The origin as browsers write it (no ":80"). */
export function serverOrigin(s: Server): string {
	return s.port === 80 ? `http://${s.host}` : `http://${s.host}:${s.port}`;
}

/** The app's address; the parameter tells the app which TV it runs on. */
export function appUrl(s: Server, platform: Platform): string {
	const base = `${serverOrigin(s)}/`;
	return platform === 'browser' ? base : `${base}?${PLATFORM_PARAM}=${platform}`;
}

export function loadServer(): Server | undefined {
	const v = load(KEY);
	if (typeof v !== 'object' || v === null) return undefined;
	const host: unknown = Reflect.get(v, 'host');
	const port: unknown = Reflect.get(v, 'port');
	if (typeof host !== 'string' || typeof port !== 'number') return undefined;
	const s = parseServer(host, String(port));
	return typeof s === 'string' ? undefined : s;
}

export function saveServer(s: Server): void {
	save(KEY, s);
}
