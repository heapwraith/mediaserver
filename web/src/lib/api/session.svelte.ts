import { load, save } from '../platform/device';
import type { Profile, Session } from './types';

const KEY = 'mediaserver.session';

interface State {
	/** The chosen profile (undefined = "Quem está assistindo?"). */
	profile?: Profile;
	/** Signed session token, sent as X-Profile ('' = none). */
	token: string;
	/** Media token for URLs the browser loads by itself (m=…). */
	mediaToken: string;
}

function isProfile(v: unknown): v is Profile {
	return (
		typeof v === 'object' &&
		v !== null &&
		'id' in v &&
		typeof v.id === 'string' &&
		'name' in v &&
		typeof v.name === 'string' &&
		'color' in v &&
		typeof v.color === 'string'
	);
}

function initial(): State {
	const saved = load(KEY);
	if (typeof saved === 'object' && saved !== null && 'token' in saved && 'profile' in saved) {
		const { token, profile } = saved;
		const mediaToken =
			'mediaToken' in saved && typeof saved.mediaToken === 'string' ? saved.mediaToken : '';
		if (typeof token === 'string' && token !== '' && isProfile(profile)) {
			return { token, profile, mediaToken };
		}
	}
	return { token: '', mediaToken: '' };
}

/** The current profile session, saved on the device. */
export const session: State = $state(initial());

export function setSession(s: Session): void {
	session.profile = s.profile;
	session.token = s.token;
	session.mediaToken = s.media_token ?? '';
	save(KEY, { profile: s.profile, token: s.token, mediaToken: session.mediaToken });
}

/** Keeps the profile and replaces the media token (it is renewed every few hours). */
export function setMediaToken(token: string): void {
	session.mediaToken = token;
	if (session.profile)
		save(KEY, { profile: session.profile, token: session.token, mediaToken: token });
}

/** Forgets the session: the app goes back to "Quem está assistindo?". */
export function clearSession(): void {
	session.profile = undefined;
	session.token = '';
	session.mediaToken = '';
	save(KEY, null);
}
