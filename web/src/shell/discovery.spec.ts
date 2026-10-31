import { describe, expect, it, vi } from 'vitest';

import { probe, scan, subnetHosts, subnetsToScan } from './discovery';

const health = (name = 'mediaserver', status = 200) =>
	Response.json({ name, status: 'ok', version: '1.2.3', db: 'ok' }, { status });

/** A network where only `servers` answer; the rest never do (until aborted). */
function network(servers: Record<string, () => Response>) {
	let inFlight = 0;
	let maxInFlight = 0;
	const fetchFn = vi.fn<typeof fetch>((input, init) => {
		const url = new URL(input instanceof Request ? input.url : input);
		inFlight++;
		maxInFlight = Math.max(maxInFlight, inFlight);
		const done = () => inFlight--;
		const answer = servers[url.host];
		if (answer !== undefined) {
			done();
			return Promise.resolve(answer());
		}
		return new Promise((_resolve, reject) => {
			init?.signal?.addEventListener('abort', () => {
				done();
				reject(new DOMException('aborted', 'AbortError'));
			});
		});
	});
	return { fetchFn, max: () => maxInFlight };
}

describe('subnetHosts', () => {
	it('lists the /24 network without the device itself', () => {
		const hosts = subnetHosts('192.168.0.7');
		expect(hosts).toHaveLength(253);
		expect(hosts[0]).toBe('192.168.0.1');
		expect(hosts.at(-1)).toBe('192.168.0.254');
		expect(hosts).not.toContain('192.168.0.7');
	});
	it('rejects what is not an IPv4', () => {
		expect(subnetHosts('')).toEqual([]);
		expect(subnetHosts('nas.local')).toEqual([]);
	});
});

describe('subnetsToScan', () => {
	it.each([
		['192.168.0.7', undefined, ['192.168.0.7']],
		['192.168.0.7', '192.168.0.10', ['192.168.0.7']],
		// The saved server moved to another network the TV also reaches.
		['192.168.0.7', '10.0.0.4', ['192.168.0.7', '10.0.0.4']],
		['', '10.0.0.4', ['10.0.0.4']],
		['', 'nas.local', ['192.168.0.0', '192.168.1.0']],
		['', undefined, ['192.168.0.0', '192.168.1.0']],
	])('device %s, saved %s', (ip, saved, want) => {
		expect(subnetsToScan(ip, saved)).toEqual(want);
	});
});

describe('probe', () => {
	it('finds a mediaserver', async () => {
		const { fetchFn } = network({ '192.168.0.10:8080': () => health() });
		await expect(probe({ host: '192.168.0.10', port: 8080 }, { fetchFn })).resolves.toEqual({
			host: '192.168.0.10',
			port: 8080,
			version: '1.2.3',
		});
		expect(fetchFn).toHaveBeenCalledWith('http://192.168.0.10:8080/api/health', expect.anything());
	});

	it('accepts a degraded server (it still answers)', async () => {
		const { fetchFn } = network({ 'nas:8080': () => health('mediaserver', 503) });
		await expect(probe({ host: 'nas', port: 8080 }, { fetchFn })).resolves.toBeDefined();
	});

	it.each([
		['another service', () => health('jellyfin')],
		['not json', () => new Response('<html>')],
	])('ignores %s', async (_name, answer) => {
		const { fetchFn } = network({ 'nas:8080': answer });
		await expect(probe({ host: 'nas', port: 8080 }, { fetchFn })).resolves.toBeUndefined();
	});

	it('gives up after the timeout', async () => {
		const { fetchFn } = network({});
		await expect(
			probe({ host: '192.168.0.99', port: 8080 }, { fetchFn, timeoutMs: 20 }),
		).resolves.toBeUndefined();
	});
});

describe('scan', () => {
	it('finds every server, a few requests at a time', async () => {
		const { fetchFn, max } = network({
			'192.168.0.3:8080': () => health(),
			'192.168.0.200:8080': () => health(),
		});
		const found: string[] = [];
		const progress: number[] = [];
		const result = await scan(subnetHosts('192.168.0.7'), 8080, {
			fetchFn,
			concurrency: 8,
			timeoutMs: 5,
			onFound: (f) => found.push(f.host),
			onProgress: (done) => progress.push(done),
		});
		expect(result.map((f) => f.host)).toEqual(['192.168.0.3', '192.168.0.200']);
		expect(found).toEqual(['192.168.0.3', '192.168.0.200']);
		expect(fetchFn).toHaveBeenCalledTimes(253);
		expect(max()).toBeLessThanOrEqual(8);
		expect(progress.at(-1)).toBe(253);
	});

	it('stops when aborted', async () => {
		const { fetchFn } = network({});
		const stop = new AbortController();
		const running = scan(subnetHosts('10.0.0.1'), 8080, {
			fetchFn,
			concurrency: 4,
			timeoutMs: 10_000,
			signal: stop.signal,
		});
		stop.abort();
		await expect(running).resolves.toEqual([]);
		expect(fetchFn.mock.calls.length).toBeLessThanOrEqual(4);
	});
});
