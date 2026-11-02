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

describe('withAdmin', () => {
	it('asks for the PIN on admin_required and tries once more', async () => {
		const { admin, finishAdmin } = await import('./admin.svelte');
		const { ApiError, withAdmin } = await import('./client');
		let calls = 0;
		const p = withAdmin(() => {
			calls++;
			if (calls === 1) return Promise.reject(new ApiError(403, 'x', 'admin_required'));
			return Promise.resolve('done');
		});
		await vi.waitFor(() => expect(admin.prompt).toBeDefined());
		finishAdmin(true);
		await expect(p).resolves.toBe('done');
		expect(calls).toBe(2);
	});

	it('gives up when the PIN pad is cancelled, and lets other errors through', async () => {
		const { admin, finishAdmin } = await import('./admin.svelte');
		const { ApiError, withAdmin } = await import('./client');
		const p = withAdmin(() => Promise.reject(new ApiError(403, 'x', 'admin_required')));
		await vi.waitFor(() => expect(admin.prompt).toBeDefined());
		finishAdmin(false);
		await expect(p).rejects.toMatchObject({ code: 'admin_required' });
		await expect(withAdmin(() => Promise.reject(new ApiError(500, 'boom')))).rejects.toMatchObject({
			status: 500,
		});
	});
});

describe('media token in URLs', () => {
	it('adds the token of the session to what the browser loads by itself', async () => {
		const { session } = await import('./session.svelte');
		const { assURL, vttURL } = await import('./client');
		const images = await import('./images');
		session.mediaToken = '';
		expect(images.imageURL('x')).toBe('/api/images/x?variant=card&w=960');
		expect(assURL('s')).toBe('/api/subtitles/s/ass');
		session.mediaToken = 'a.b.c+/=';
		expect(images.imageURL('x')).toBe('/api/images/x?variant=card&w=960&m=a.b.c%2B%2F%3D');
		expect(images.imageSrcset('x')).toContain('&m=a.b.c%2B%2F%3D 480w');
		expect(vttURL('s', 2)).toBe('/api/subtitles/s/vtt?offset=2&m=a.b.c%2B%2F%3D');
		expect(assURL('s')).toBe('/api/subtitles/s/ass?m=a.b.c%2B%2F%3D');
		session.mediaToken = '';
	});
});
