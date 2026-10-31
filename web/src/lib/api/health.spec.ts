import { describe, expect, it, vi } from 'vitest';

import { fetchHealth } from './health';

function fakeFetch(status: number, body: unknown): typeof fetch {
	return vi.fn<typeof fetch>(async () => Response.json(body, { status }));
}

describe('fetchHealth', () => {
	it('returns the health body', async () => {
		const health = { status: 'ok', version: '1.0.0', db: 'ok' };
		const fetchFn = fakeFetch(200, health);
		await expect(fetchHealth(fetchFn)).resolves.toEqual(health);
		expect(fetchFn).toHaveBeenCalledWith('/api/health', expect.anything());
	});

	it('accepts the degraded (503) answer', async () => {
		const health = { status: 'degraded', version: '1.0.0', db: 'error' };
		await expect(fetchHealth(fakeFetch(503, health))).resolves.toEqual(health);
	});

	it('rejects an unexpected body', async () => {
		await expect(fetchHealth(fakeFetch(200, { nope: true }))).rejects.toThrow('unexpected');
	});
});
