import {
	getProgress,
	getSeries,
	movieExtras,
	seriesExtras,
	seriesProgress,
	streamURL,
} from '../api/client';
import type { Episode, Extra, Movie, Progress, SeriesDetail, SeriesProgress } from '../api/types';
import { episodeTitle, formatDuration, playHref, resolution, seasonEpisode } from './cards';
import { remaining, resumeAt } from './progress';

/** The main button of a hero: "▶ Continuar" (with "Do início") or "▶ Assistir". */
export interface PlayAction {
	href: string;
	label: string;
	/** "Do início": shown when the item can be resumed. */
	restart?: string;
}

/** What a hero shows and where its buttons go. */
export interface HeroData {
	title: string;
	meta: string;
	imageId: string;
	color?: string;
	play?: PlayAction;
	trailer?: Extra;
	trailerSrc?: string;
	details: string;
	extras: Extra[];
}

const join = (parts: (string | number | undefined)[]) => parts.filter(Boolean).join(' · ');

/** Episodes in viewing order (specials last), with their season number. */
export function episodesInOrder(s: SeriesDetail): { season: number; episode: Episode }[] {
	const seasons = [...s.seasons].sort((a, b) =>
		a.number === 0 ? 1 : b.number === 0 ? -1 : a.number - b.number,
	);
	return seasons.flatMap((se) => se.episodes.map((episode) => ({ season: se.number, episode })));
}

export function moviePlay(m: Movie, p?: Progress): PlayAction {
	const href = playHref(m.media_id, m.title);
	if (p !== undefined && resumeAt(p) > 0) {
		return {
			href,
			label: `▶ Continuar · ${remaining(p)}`,
			restart: playHref(m.media_id, m.title, true),
		};
	}
	return { href, label: '▶ Assistir' };
}

/** Plays the up-next episode of the profile, else the first one. */
export function seriesPlay(s: SeriesDetail, sp?: SeriesProgress): PlayAction | undefined {
	const all = episodesInOrder(s);
	const next =
		sp?.up_next === undefined ? undefined : all.find((x) => x.episode.media_id === sp.up_next);
	const pick = next ?? all[0];
	if (pick === undefined) return undefined;
	const { episode: e } = pick;
	const title = episodeTitle(s.title, e);
	const href = playHref(e.media_id, title);
	const where = seasonEpisode(s.seasons.length > 1 ? pick.season : undefined, e);
	const p = sp?.episodes[e.media_id];
	if (p !== undefined && resumeAt(p) > 0) {
		return {
			href,
			label: `▶ Continuar ${where} · ${remaining(p)}`,
			restart: playHref(e.media_id, title, true),
		};
	}
	return { href, label: next === undefined ? '▶ Assistir' : `▶ Assistir ${where}` };
}

export async function movieHero(m: Movie): Promise<HeroData & { progress?: Progress }> {
	const [extras, progress] = await Promise.all([
		movieExtras(m.id).catch(() => []),
		getProgress(m.media_id).catch(() => undefined),
	]);
	const trailer = extras.find((x) => x.type === 'trailer');
	return {
		title: m.title,
		meta: join(['Filme', m.year, formatDuration(m.duration_ms), resolution(m.height)]),
		imageId: m.id,
		color: m.color,
		play: moviePlay(m, progress),
		trailer,
		trailerSrc: trailer === undefined ? undefined : streamURL(trailer.media_id),
		details: `/filme/${m.id}`,
		extras: extras.filter((x) => x !== trailer),
		progress,
	};
}

export async function seriesHero(
	id: string,
): Promise<HeroData & { detail: SeriesDetail; progress?: SeriesProgress }> {
	const [detail, extras, progress] = await Promise.all([
		getSeries(id),
		seriesExtras(id).catch(() => []),
		seriesProgress(id).catch(() => undefined),
	]);
	const first = episodesInOrder(detail)[0]?.episode;
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
		play: seriesPlay(detail, progress),
		trailer,
		trailerSrc: trailer === undefined ? undefined : streamURL(trailer.media_id),
		details: `/serie/${detail.id}`,
		extras: extras.filter((x) => x !== trailer),
		detail,
		progress,
	};
}
