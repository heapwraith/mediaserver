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

/** How to play a file (internal/stream/transcode.go Playback). */
export interface Playback {
	/** The file itself (true) or an HLS session. */
	direct: boolean;
	url: string;
	/** Seconds: media time of the stream's time 0 (HLS), or where to seek (direct). */
	start: number;
	video?: 'copy' | 'h264';
	audio?: 'copy' | 'aac';
	burned?: boolean;
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
	video_bits?: number;
	audio_codec?: string;
	hdr?: string;
	audio: Track[];
	subtitles: Subtitle[];
}

export interface Subtitle {
	id: string;
	language?: string;
	codec: string;
	kind: 'text' | 'ass' | '';
	title?: string;
	forced?: boolean;
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

// Profiles and progress (internal/library/profiles.go, progress.go).

export interface Profile {
	id: string;
	name: string;
	color: string;
	has_pin: boolean;
	created_at: number;
	subtitle_language: string;
}

/** A chosen profile: `token` goes in the X-Profile header. */
export interface Session {
	profile: Profile;
	token: string;
}

/** On update, a missing pin keeps it, "" removes it and 4 digits set it. */
export interface ProfileInput {
	name: string;
	color: string;
	pin?: string;
	subtitle_language?: string;
}

export interface Progress {
	media_id: string;
	position_ms: number;
	duration_ms: number;
	watched: boolean;
	updated_at?: number;
}

export interface SeriesProgress {
	/** Progress by media id (only the episodes played). */
	episodes: Record<string, Progress>;
	/** Media id of the episode to play next. */
	up_next?: string;
}

/** A card of "Continuar assistindo" / "Próximos episódios", or the next episode. */
export interface HomeItem {
	media_id: string;
	kind: 'movie' | 'episode';
	/** Movie or series title. */
	title: string;
	/** Movie or series id (the details page). */
	parent_id: string;
	series_kind?: SeriesKind;
	season?: number;
	episode?: Episode;
	position_ms: number;
	duration_ms: number;
	updated_at?: number;
	color?: string;
}

export interface Home {
	continue: HomeItem[];
	next: HomeItem[];
}

/** Exactly one of media_ids, season_id or series_id. */
export interface WatchedTarget {
	media_ids?: string[];
	season_id?: string;
	series_id?: string;
	watched: boolean;
}

/** A running FFmpeg session (GET /api/admin/status). */
export interface SessionStatus {
	media_id: string;
	file: string;
	video: string; // copy | h264
	audio: string; // copy | aac
	burned: boolean;
	encode: boolean;
	started: string;
	last_request?: string;
}

export interface ServerStatus {
	version: string;
	started_at: string;
	uptime_s: number;
	cpu: { process: number; system: number | null };
	memory: { bytes: number; heap: number };
	encoder: string;
	gpus: { name: string; utilization: number; encoder: number }[] | null;
	sessions: SessionStatus[];
	slots: { used: number; limit: number };
	disk: {
		transcode: number;
		transcode_limit: number;
		images: number;
		subtitles: number;
		database: number;
	};
	library: { movies: number; series: number; animes: number; files: number; review: number };
}
