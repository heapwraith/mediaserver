import type { Episode, Extra, Movie, Series } from '../api/types';

/** What a card shows; `imageId` is any id /api/images accepts. */
export interface CardItem {
	key: string;
	imageId: string;
	title: string;
	subtitle?: string;
	color?: string;
	href: string;
}

export function formatDuration(ms?: number): string {
	if (ms === undefined || ms <= 0) return '';
	const min = Math.round(ms / 60_000);
	return min < 60 ? `${min} min` : `${Math.floor(min / 60)} h ${min % 60} min`;
}

export function resolution(height?: number): string {
	if (height === undefined) return '';
	if (height >= 2000) return '4K';
	if (height >= 1000) return '1080p';
	if (height >= 700) return '720p';
	return 'SD';
}

export const movieCard = (m: Movie): CardItem => ({
	key: m.id,
	imageId: m.id,
	title: m.title,
	subtitle: [m.year, formatDuration(m.duration_ms)].filter(Boolean).join(' · '),
	color: m.color,
	href: `/filme/${m.id}`,
});

export const seriesCard = (s: Series): CardItem => ({
	key: s.id,
	imageId: s.id,
	title: s.title,
	subtitle:
		s.seasons > 1
			? `${s.seasons} temporadas`
			: `${s.episodes} episódio${s.episodes === 1 ? '' : 's'}`,
	color: s.color,
	href: `/serie/${s.id}`,
});

export function episodeLabel(e: Episode): string {
	if (e.number === undefined)
		return e.absolute_number === undefined ? '' : `Ep. ${e.absolute_number}`;
	return e.number_end === undefined ? `Ep. ${e.number}` : `Ep. ${e.number}–${e.number_end}`;
}

/** Player URL; `back` is where Voltar returns. */
export const playHref = (mediaId: string, title: string) =>
	`/assistir/${mediaId}?title=${encodeURIComponent(title)}`;

const extraTypes: Record<Extra['type'], string> = {
	trailer: 'Trailer',
	featurette: 'Featurette',
	behind_the_scenes: 'Bastidores',
	deleted: 'Cena excluída',
	interview: 'Entrevista',
	other: 'Extra',
};

/** Trailers and extras play straight from their media id. */
export const extraCard = (x: Extra): CardItem => ({
	key: x.id,
	imageId: x.media_id,
	title: x.title,
	subtitle: [
		extraTypes[x.type],
		x.season === undefined ? '' : `Temporada ${x.season}`,
		formatDuration(x.duration_ms),
	]
		.filter(Boolean)
		.join(' · '),
	href: playHref(x.media_id, x.title),
});
