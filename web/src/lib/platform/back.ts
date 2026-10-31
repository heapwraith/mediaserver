import { goto, type AfterNavigate } from '$app/navigation';

// Pages visited inside the app: Voltar never leaves it (a page opened from a
// link has nothing to go back to but Início).
let depth = 0;

/** Counts in-app navigations; call it from afterNavigate. */
export function trackNavigation(nav: AfterNavigate): void {
	if (nav.type === 'popstate') depth = Math.max(0, depth + (nav.delta ?? -1));
	else if (nav.type === 'link' || nav.type === 'goto') depth += 1;
}

/**
 * Voltar closes the current level: player → details → menu page → Início.
 * `top` are the menu pages, which go back to Início. On Início, `atHome`
 * runs (TVs ask whether to leave the app; the browser has nothing to close).
 */
export function goBack(top: ReadonlySet<string> = new Set(), atHome?: () => void): void {
	const path = location.pathname;
	if (path === '/') {
		atHome?.();
		return;
	}
	if (top.has(path)) void goto('/');
	else if (depth > 0) history.back();
	else void goto('/', { replaceState: true });
}
