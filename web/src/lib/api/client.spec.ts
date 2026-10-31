import { describe, expect, it, vi } from 'vitest';

import { api, query } from './client';
import { imageSrcset } from './images';

describe('api', () => {
	it('decodes the JSON body', async () => {
		const fetchFn = vi.fn<typeof fetch>(async () => Response.json({ a: 1 }));
		await expect(api('/api/x', {}, fetchFn)).resolves.toEqual({ a: 1 });
	});

	it('turns {"error"} answers into ApiError', async () => {
		const fetchFn = vi.fn<typeof fetch>(async () =>
			Response.json({ error: 'not found', code: 'not_found' }, { status: 404 }),
		);
		await expect(api('/api/x', {}, fetchFn)).rejects.toMatchObject({
			status: 404,
			message: 'not found',
			code: 'not_found',
		});
	});

	it('reports network failures as status 0', async () => {
		const fetchFn = vi.fn<typeof fetch>(async () => Promise.reject(new TypeError('offline')));
		await expect(api('/api/x', {}, fetchFn)).rejects.toMatchObject({ status: 0, code: 'offline' });
	});
});

describe('query', () => {
	it('drops empty values', () => {
		expect(query({ sort: 'added', order: 'desc', q: '', limit: 20 })).toBe(
			'?sort=added&order=desc&limit=20',
		);
		expect(query({})).toBe('');
	});
});

describe('imageSrcset', () => {
	it('lists every width', () => {
		expect(imageSrcset('abc', 'backdrop')).toBe(
			'/api/images/abc?variant=backdrop&w=480 480w, /api/images/abc?variant=backdrop&w=960 960w, /api/images/abc?variant=backdrop&w=1920 1920w',
		);
	});
});
