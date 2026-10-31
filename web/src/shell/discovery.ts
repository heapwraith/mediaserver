// Finds mediaservers on the local network. TV web apps cannot send UDP
// (SSDP/mDNS), so the shell asks GET /api/health of every address of the
// TV's /24 network, a few at a time (the endpoint allows any origin).

import { isIPv4 } from '../lib/platform/native';
import { serverOrigin, type Server } from './server';

export interface Found extends Server {
	version: string;
}

interface ProbeOptions {
	fetchFn?: typeof fetch;
	timeoutMs?: number;
	/** Stops the request early (the whole scan was cancelled). */
	signal?: AbortSignal;
}

/** The other addresses of the /24 network of `ip`. */
export function subnetHosts(ip: string): string[] {
	if (!isIPv4(ip)) return [];
	const prefix = ip.slice(0, ip.lastIndexOf('.'));
	const hosts: string[] = [];
	for (let i = 1; i <= 254; i++) {
		const host = `${prefix}.${i}`;
		if (host !== ip) hosts.push(host);
	}
	return hosts;
}

const sameSubnet = (a: string, b: string) =>
	a.slice(0, a.lastIndexOf('.')) === b.slice(0, b.lastIndexOf('.'));

/**
 * The addresses whose /24 network to scan: the device's, and the saved
 * server's when it is elsewhere. Unknown device IP and no saved IPv4: the
 * two networks home routers use most.
 */
export function subnetsToScan(localIp: string, savedHost?: string): string[] {
	const out: string[] = [];
	if (isIPv4(localIp)) out.push(localIp);
	if (
		savedHost !== undefined &&
		isIPv4(savedHost) &&
		!out.some((ip) => sameSubnet(ip, savedHost))
	) {
		out.push(savedHost);
	}
	return out.length > 0 ? out : ['192.168.0.0', '192.168.1.0'];
}

/** Asks one address whether it is a mediaserver. */
export async function probe(server: Server, opts: ProbeOptions = {}): Promise<Found | undefined> {
	const { fetchFn = fetch, timeoutMs = 1500, signal } = opts;
	const ctrl = new AbortController();
	const stop = () => ctrl.abort();
	const timer = setTimeout(stop, timeoutMs);
	signal?.addEventListener('abort', stop);
	try {
		const res = await fetchFn(`${serverOrigin(server)}/api/health`, {
			signal: ctrl.signal,
			cache: 'no-store',
		});
		const body: unknown = await res.json();
		if (typeof body !== 'object' || body === null) return undefined;
		const version: unknown = Reflect.get(body, 'version');
		if (Reflect.get(body, 'name') !== 'mediaserver' || typeof version !== 'string')
			return undefined;
		return { host: server.host, port: server.port, version };
	} catch {
		return undefined; // nothing there, another service, or too slow
	} finally {
		clearTimeout(timer);
		signal?.removeEventListener('abort', stop);
	}
}

interface ScanOptions extends ProbeOptions {
	/** Requests at a time (TV network stacks dislike hundreds at once). */
	concurrency?: number;
	onFound?: (f: Found) => void;
	/** Addresses answered or given up so far. */
	onProgress?: (done: number, total: number) => void;
}

/** Probes `hosts` on `port`; servers come out in address order. */
export async function scan(
	hosts: string[],
	port: number,
	opts: ScanOptions = {},
): Promise<Found[]> {
	const { concurrency = 24, onFound, onProgress, signal } = opts;
	const found: (Found | undefined)[] = [];
	let next = 0;
	let done = 0;
	const stopped = () => signal?.aborted === true;
	/** Takes the next address until none is left (a few workers share the list). */
	async function worker(): Promise<void> {
		if (next >= hosts.length || stopped()) return;
		const i = next++;
		const f = await probe({ host: hosts[i] ?? '', port }, opts);
		if (stopped()) return;
		found[i] = f;
		if (f !== undefined) onFound?.(f);
		onProgress?.(++done, hosts.length);
		return worker();
	}
	await Promise.all(Array.from({ length: Math.min(concurrency, hosts.length) }, worker));
	return found.filter((f): f is Found => f !== undefined);
}
