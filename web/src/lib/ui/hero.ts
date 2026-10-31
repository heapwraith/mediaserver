import { getSeries, movieExtras, seriesExtras, streamURL } from '../api/client';
import type { Extra, Movie, SeriesDetail } from '../api/types';
import { episodeLabel, formatDuration, playHref, resolution } from './cards';

/** What a hero shows and where its buttons go. */
export interface HeroData {
	title: string;
	meta: string;
	imageId: string;
	color?: string;
	play?: string;
	trailer?: Extra;
	trailerSrc?: string;
	details: string;
	extras: Extra[];
}

const join = (parts: (string | number | undefined)[]) => parts.filter(Boolean).join(' · ');

export function firstEpisode(s: SeriesDetail) {
	const seasons = [...s.seasons].sort((a, b) =>
		a.number === 0 ? 1 : b.number === 0 ? -1 : a.number - b.number,
	);
	return seasons[0]?.episodes[0];
}

export async function movieHero(m: Movie): Promise<HeroData> {
	const extras = await movieExtras(m.id).catch(() => []);
	const trailer = extras.find((x) => x.type === 'trailer');
	return {
		title: m.title,
		meta: join(['Filme', m.year, formatDuration(m.duration_ms), resolution(m.height)]),
		imageId: m.id,
		color: m.color,
		play: playHref(m.media_id, m.title),
		trailer,
		trailerSrc: trailer === undefined ? undefined : streamURL(trailer.media_id),
		details: `/filme/${m.id}`,
		extras: extras.filter((x) => x !== trailer),
	};
}

export async function seriesHero(id: string): Promise<HeroData & { detail: SeriesDetail }> {
	const [detail, extras] = await Promise.all([getSeries(id), seriesExtras(id).catch(() => [])]);
	const first = firstEpisode(detail);
	const trailer = extras.find((x) => x.type === 'trailer');
	const n = detail.seasons.length;
	return {
		title: detail.title,
		meta: join([
			detail.kind === 'anime' ? 'Anime' : 'Série',
			detail.year,
			n > 1 ? `${n} temporadas` : `${detail.episodes} episódio${detail.episodes === 1 ? '' : 's'}`,
			resolution(first?.height),
		]),
		imageId: detail.id,
		color: detail.color,
		play:
			first === undefined
				? undefined
				: playHref(first.media_id, `${detail.title} · ${episodeLabel(first)}`),
		trailer,
		trailerSrc: trailer === undefined ? undefined : streamURL(trailer.media_id),
		details: `/serie/${detail.id}`,
		extras: extras.filter((x) => x !== trailer),
		detail,
	};
}
