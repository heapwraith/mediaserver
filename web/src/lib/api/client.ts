import { clearSession, session } from './session.svelte';
import type {
	Correction,
	Extra,
	Home,
	HomeItem,
	List,
	Media,
	Movie,
	Playback,
	Profile,
	ProfileInput,
	Progress,
	ReviewItem,
	ScanStatus,
	Series,
	SeriesDetail,
	SeriesKind,
	SeriesProgress,
	Session,
	WatchedTarget,
} from './types';

/** Header of the profile session (internal/api/session.go). */
export const PROFILE_HEADER = 'X-Profile';

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
		if (session.token !== '') headers.set(PROFILE_HEADER, session.token);
		res = await fetchFn(path, { ...init, headers });
	} catch {
		throw new ApiError(0, 'servidor indisponível');
	}
	const body: unknown = await res.json().catch(() => null);
	// The session ended (profile deleted, PIN changed): choose a profile again.
	if (res.status === 401) clearSession();
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
/** Direct play or HLS, decided by the server from what this device plays. */
export const startPlayback = (mediaId: string, caps: string[], audio: number, start: number) =>
	api<Playback>(`/api/media/${mediaId}/playback`, {
		method: 'POST',
		body: JSON.stringify({ caps, audio, start }),
	});
/** Stops an HLS session (its FFmpeg); keepalive survives the page closing. */
export const stopPlayback = (p: Playback) =>
	api<unknown>(p.url.replace(/\/index\.m3u8$/, ''), { method: 'DELETE', keepalive: true });

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

const json = (method: string, body: unknown, keepalive = false): RequestInit => ({
	method,
	body: JSON.stringify(body),
	keepalive,
});

export const listProfiles = () => api<Profile[]>('/api/profiles');
export const createProfile = (p: ProfileInput) => api<Session>('/api/profiles', json('POST', p));
export const startSession = (id: string, pin?: string) =>
	api<Session>(`/api/profiles/${id}/session`, json('POST', pin === undefined ? {} : { pin }));
export const updateProfile = (id: string, p: ProfileInput) =>
	api<Session>(`/api/profiles/${id}`, json('PUT', p));
export const deleteProfile = (id: string) =>
	api<unknown>(`/api/profiles/${id}`, { method: 'DELETE' });

export const getHome = () => api<Home>('/api/home');
export const getProgress = (mediaId: string) => api<Progress>(`/api/progress/${mediaId}`);
/** keepalive: the request survives the page closing (pagehide). */
export const saveProgress = (
	mediaId: string,
	positionMs: number,
	durationMs: number,
	keepalive = false,
) =>
	api<Progress>(
		`/api/progress/${mediaId}`,
		json(
			'PUT',
			{ position_ms: Math.round(positionMs), duration_ms: Math.round(durationMs) },
			keepalive,
		),
	);
export const setWatched = (t: WatchedTarget) => api<unknown>('/api/watched', json('PUT', t));
export const seriesProgress = (id: string) => api<SeriesProgress>(`/api/series/${id}/progress`);
export const nextEpisode = (mediaId: string) => api<HomeItem>(`/api/media/${mediaId}/next`);
