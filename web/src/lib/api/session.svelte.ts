import { load, save } from '../platform/device';
import type { Profile, Session } from './types';

const KEY = 'mediaserver.session';

interface State {
	/** The chosen profile (undefined = "Quem está assistindo?"). */
	profile?: Profile;
	/** Signed session token, sent as X-Profile ('' = none). */
	token: string;
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
		if (typeof token === 'string' && token !== '' && isProfile(profile)) return { token, profile };
	}
	return { token: '' };
}

/** The current profile session, saved on the device. */
export const session: State = $state(initial());

export function setSession(s: Session): void {
	session.profile = s.profile;
	session.token = s.token;
	save(KEY, { profile: s.profile, token: s.token });
}

/** Forgets the session: the app goes back to "Quem está assistindo?". */
export function clearSession(): void {
	session.profile = undefined;
	session.token = '';
	save(KEY, null);
}
