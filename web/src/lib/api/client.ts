import type {
	Correction,
	Extra,
	List,
	Media,
	Movie,
	ReviewItem,
	ScanStatus,
	Series,
	SeriesDetail,
	SeriesKind,
} from './types';

/** An error answered by the API ({"error": "..."}), or a network failure (status 0). */
export class ApiError extends Error {
	constructor(
		readonly status: number,
		message: string,
	) {
		super(message);
		this.name = 'ApiError';
	}
}

/** Calls the API and decodes its JSON. The body shape is trusted (same binary). */
export async function api<T>(
	path: string,
	init: RequestInit = {},
	fetchFn: typeof fetch = fetch,
): Promise<T> {
	let res: Response;
	try {
		const headers = new Headers(init.headers);
		headers.set('Accept', 'application/json');
		if (init.body !== undefined) headers.set('Content-Type', 'application/json');
		res = await fetchFn(path, { ...init, headers });
	} catch {
		throw new ApiError(0, 'servidor indisponível');
	}
	const body: unknown = await res.json().catch(() => null);
	if (!res.ok) {
		const message =
			typeof body === 'object' && body !== null && 'error' in body && typeof body.error === 'string'
				? body.error
				: `HTTP ${res.status}`;
		throw new ApiError(res.status, message);
	}
	// The API is served by this same binary: its shapes are lib/api/types.ts.
	// oxlint-disable-next-line typescript/no-unsafe-type-assertion
	return body as T;
}

export interface ListParams {
	sort?: 'title' | 'added' | 'year';
	order?: 'asc' | 'desc';
	offset?: number;
	limit?: number;
	q?: string;
}

/** Query string of a listing, without empty values. */
export function query(p: ListParams): string {
	const q = new URLSearchParams();
	for (const [k, v] of Object.entries(p)) {
		if (v !== undefined && v !== '') q.set(k, String(v));
	}
	const s = q.toString();
	return s === '' ? '' : `?${s}`;
}

const seriesPath = (kind: SeriesKind) => (kind === 'anime' ? '/api/animes' : '/api/series');

export const listMovies = (p: ListParams = {}) => api<List<Movie>>(`/api/movies${query(p)}`);
export const listSeries = (kind: SeriesKind, p: ListParams = {}) =>
	api<List<Series>>(`${seriesPath(kind)}${query(p)}`);
export const getMovie = (id: string) => api<Movie>(`/api/movies/${id}`);
export const getSeries = (id: string) => api<SeriesDetail>(`/api/series/${id}`);
export const movieExtras = (id: string) => api<Extra[]>(`/api/movies/${id}/extras`);
export const seriesExtras = (id: string) => api<Extra[]>(`/api/series/${id}/extras`);
export const getMedia = (id: string) => api<Media>(`/api/media/${id}`);
export const streamURL = (mediaId: string) => `/api/media/${mediaId}/stream`;

export const scanStatus = () => api<ScanStatus>('/api/library/scan');
export const startScan = () => api<{ started: boolean }>('/api/library/scan', { method: 'POST' });
export const reviewQueue = () => api<ReviewItem[]>('/api/review');
export const correctFile = (id: string, c: Correction) =>
	api<unknown>(`/api/review/${id}`, { method: 'PUT', body: JSON.stringify(c) });
export const mergeSeries = (sourceIds: string[], targetId: string) =>
	api<{ series_id: string }>('/api/series/merge', {
		method: 'POST',
		body: JSON.stringify({ source_ids: sourceIds, target_id: targetId }),
	});
export const splitSeries = (seriesId: string, fileIds: string[], title: string) =>
	api<{ series_id: string }>('/api/series/split', {
		method: 'POST',
		body: JSON.stringify({ series_id: seriesId, file_ids: fileIds, title }),
	});
