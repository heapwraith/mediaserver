// Progress rules shared by the cards, the details pages and the player.
// The server decides "watched" (90%, internal/library/progress.go).

import type { Marker, Progress } from '../api/types';

/** Below this position an item is not "started" (same as the server). */
export const MIN_RESUME_S = 30;
/** The player saves the position this often while playing. */
export const SAVE_EVERY_MS = 10_000;
/** The player does not save before this position (opened by mistake). */
export const MIN_SAVE_S = 5;
/** The "Próximo episódio" card shows in the last seconds. */
export const NEXT_CARD_S = 15;

/** Where to resume, in seconds (0 = from the start). */
export function resumeAt(p?: Progress): number {
	if (p === undefined || p.watched || p.position_ms < MIN_RESUME_S * 1000) return 0;
	return p.position_ms / 1000;
}

/** Fraction watched for a progress bar, or undefined when there is none. */
export function fraction(p?: Progress): number | undefined {
	if (p === undefined || p.watched || p.duration_ms <= 0 || p.position_ms <= 0) return undefined;
	return Math.min(1, p.position_ms / p.duration_ms);
}

/** "faltam 32 min", "falta 1 min" (never 0). */
export function remaining(p: Pick<Progress, 'position_ms' | 'duration_ms'>): string {
	const min = Math.max(1, Math.round((p.duration_ms - p.position_ms) / 60_000));
	if (min >= 60) return `faltam ${Math.floor(min / 60)} h ${min % 60} min`;
	return min === 1 ? 'falta 1 min' : `faltam ${min} min`;
}

/** Whether the player saves now: playing, past MIN_SAVE_S, SAVE_EVERY_MS after the last save. */
export function shouldSave(timeS: number, lastSaveMs: number, nowMs: number): boolean {
	return timeS >= MIN_SAVE_S && nowMs - lastSaveMs >= SAVE_EVERY_MS;
}

/** Where "Pular abertura" jumps to (s) while the opening plays, or null (not in its last 2 s). */
export function skipIntro(timeS: number, markers: Marker[] = []): number | null {
	const m = markers.find((k) => k.kind === 'intro');
	if (m === undefined) return null;
	const end = m.end_ms / 1000;
	return timeS >= m.start_ms / 1000 && timeS < end - 2 ? end : null;
}

/** When the end credits start (s), if the server found them. */
export function creditsAt(markers: Marker[] = []): number | undefined {
	const m = markers.find((k) => k.kind === 'credits');
	return m === undefined ? undefined : m.start_ms / 1000;
}

/**
 * Seconds left on the "Próximo episódio" countdown, or null while the card
 * is hidden (too early, or a video too short for it). With known credits the
 * card shows when they start and counts NEXT_CARD_S from there.
 */
export function nextCountdown(timeS: number, durationS: number, creditsS?: number): number | null {
	if (durationS <= NEXT_CARD_S * 2) return null;
	if (creditsS !== undefined && timeS >= creditsS) {
		return Math.max(0, Math.ceil(creditsS + NEXT_CARD_S - timeS));
	}
	const left = durationS - timeS;
	return left <= NEXT_CARD_S ? Math.max(0, Math.ceil(left)) : null;
}
