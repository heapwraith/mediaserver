// The administrator session: the token of POST /api/admin/unlock (kept only in
// memory, so it ends with the app) and the PIN prompt the layout shows.

interface State {
	/** Sent as X-Admin ('' = none). */
	token: string;
	/** Set while the PIN pad is on screen. */
	prompt?: { resolve: (ok: boolean) => void };
}

export const admin: State = $state({ token: '' });

/** Opens the PIN pad; resolves true once the PIN was accepted, false on Cancelar. */
export function askAdmin(): Promise<boolean> {
	admin.prompt?.resolve(false);
	return new Promise((resolve) => {
		admin.prompt = { resolve };
	});
}

export function finishAdmin(ok: boolean): void {
	const p = admin.prompt;
	admin.prompt = undefined;
	p?.resolve(ok);
}
