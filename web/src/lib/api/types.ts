// Mirrors of the JSON answered by the Go API (internal/library, internal/scanner).

export interface List<T> {
	items: T[];
	total: number;
	offset: number;
	limit: number;
}

export interface Movie {
	id: string;
	title: string;
	year?: number;
	media_id: string;
	duration_ms?: number;
	width?: number;
	height?: number;
	added_at: number;
	color?: string;
}

export type SeriesKind = 'series' | 'anime';

export interface Series {
	id: string;
	kind: SeriesKind;
	title: string;
	year?: number;
	seasons: number;
	episodes: number;
	added_at: number;
	color?: string;
}

export interface Episode {
	id: string;
	number?: number;
	number_end?: number;
	absolute_number?: number;
	title?: string;
	media_id: string;
	duration_ms?: number;
	width?: number;
	height?: number;
	added_at: number;
	color?: string;
}

export interface Season {
	id: string;
	number: number;
	episodes: Episode[];
}

export interface SeriesDetail extends Omit<Series, 'seasons'> {
	seasons: Season[];
}

export interface Extra {
	id: string;
	type: 'trailer' | 'featurette' | 'behind_the_scenes' | 'deleted' | 'interview' | 'other';
	title: string;
	season?: number;
	media_id: string;
	duration_ms?: number;
}

export interface Track {
	index: number;
	codec: string;
	language?: string;
	title?: string;
	channels?: number;
	default?: boolean;
}

export interface Media {
	id: string;
	kind: string;
	status: string;
	file_name: string;
	size: number;
	container?: string;
	duration_ms?: number;
	width?: number;
	height?: number;
	video_codec?: string;
	audio_codec?: string;
	hdr?: string;
	audio: Track[];
	subtitles: { id: string; language?: string; codec: string; title?: string }[];
}

export interface ScanStatus {
	running: boolean;
	started_at?: string;
	finished_at?: string;
	duration_ms: number;
	seen: number;
	added: number;
	updated: number;
	moved: number;
	removed: number;
	unchanged: number;
	pending: number;
	review: number;
	ignored: number;
	probe_errors: number;
	last_error?: string;
}

export interface Guess {
	kind?: string;
	title?: string;
	year?: number;
	season?: number;
	episode?: number;
	absolute?: number;
}

export interface ReviewItem {
	id: string;
	rel_path: string;
	size: number;
	reason: string;
	guess: Guess;
}

export type CorrectionKind = 'movie' | 'series' | 'anime' | 'ignored';

export interface Correction {
	kind: CorrectionKind;
	title?: string;
	year?: number;
	season?: number;
	episode?: number;
}
