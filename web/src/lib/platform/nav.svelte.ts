// Spatial navigation (arrows / remote) on top of lrud (BBC).
//
// The lrud tree is rebuilt from the DOM (document order) whenever focusable
// elements mount or unmount: registration order then always matches the
// screen, whatever order Svelte mounts components in. Rows and grids are a
// single lrud node each and move their own index (see lib/ui/move.ts); that
// keeps the tree small and lets them virtualize their cards.

import { Lrud, type Node, type Orientation } from 'lrud';

import type { Action } from './keys';

export interface FocusableOptions {
	/** Stable id: focus is restored on it when the user comes back to the page. */
	id?: string;
	/** Handles a key while focused; returns true when it consumed it. */
	onKey?: (a: Action) => boolean;
}

interface Item {
	el: HTMLElement;
	onKey?: (a: Action) => boolean;
}

const items = new Map<string, Item>();
const activeChild = new Map<string, string>(); // group id → its last focused child
const savedFocus = new Map<string, string>(); // page path → focused id
let lrud: Lrud | undefined;
let dirty = true;
let scheduled = false;
// The focus was placed by default (not by the user): it moves to the page
// content as soon as content appears (pages that load their data later).
let byDefault = false;
let seq = 0;

/** Id of the focused element ('' when none). */
export const nav = $state({ focused: '' });

function nextId(prefix: string): string {
	seq += 1;
	return `${prefix}${seq}`;
}

function build(): Lrud {
	const l = new Lrud();
	l.registerNode('root', { orientation: 'vertical' });
	for (const el of document.querySelectorAll<HTMLElement>('[data-nav]')) {
		const id = el.dataset.navId ?? '';
		if (id === '' || l.getNode(id) !== undefined) continue;
		const parent = el.parentElement?.closest<HTMLElement>('[data-nav="group"]')?.dataset.navId;
		if (el.dataset.nav === 'group') {
			l.registerNode(id, {
				parent: parent ?? 'root',
				orientation: el.dataset.navOrientation === 'horizontal' ? 'horizontal' : 'vertical',
			});
		} else if (items.has(id)) {
			l.registerNode(id, { parent: parent ?? 'root', isFocusable: true });
		}
	}
	for (const [group, child] of activeChild) {
		if (l.getNode(group) !== undefined && l.getNode(child) !== undefined) {
			l.setActiveChild(group, child);
		}
	}
	l.on('active', (node: Node) => {
		if (node.parent !== undefined) activeChild.set(node.parent.id, node.id);
	});
	l.on('focus', (node: Node) => apply(node.id));
	return l;
}

function apply(id: string): void {
	items.get(nav.focused)?.el.removeAttribute('data-focused');
	nav.focused = id;
	savedFocus.set(location.pathname, id);
	const el = items.get(id)?.el;
	if (el === undefined) return;
	// Styled globally (app.css): scale + shadow + outline, AGENTS.md §7.
	el.setAttribute('data-focused', '');
	if (document.activeElement !== el) el.focus({ preventScroll: true });
	el.scrollIntoView({ block: 'nearest', inline: 'nearest' });
}

/** The lrud tree, rebuilt when the DOM changed; focus is kept valid. */
function tree(): Lrud {
	if (lrud === undefined || dirty) {
		dirty = false;
		lrud = build();
		const l = lrud;
		const first = document.querySelector<HTMLElement>('main [data-nav="item"]')?.dataset.navId;
		const content = first !== undefined && l.getNode(first) !== undefined ? first : undefined;
		const anyItem = [...items.keys()].find((id) => l.getNode(id) !== undefined);
		const want = [nav.focused, savedFocus.get(location.pathname) ?? ''].find(
			(id) => items.has(id) && l.getNode(id) !== undefined,
		);
		if (want !== undefined && !(byDefault && content !== undefined && want !== content)) {
			lrud.assignFocus(want);
		} else if (content !== undefined) {
			// Default: first focusable of the page content, else of the whole screen.
			lrud.assignFocus(content);
			byDefault = false;
		} else if (anyItem !== undefined) {
			try {
				lrud.assignFocus('root'); // digs down to the first focusable
			} catch {
				// The remembered group is empty (its page is still mounting):
				// lrud throws instead of trying the next one.
				lrud.assignFocus(anyItem);
			}
			byDefault = true;
		} else {
			nav.focused = '';
		}
	}
	return lrud;
}

/** Rebuilds on the next frame (many elements mount together). */
function changed(): void {
	dirty = true;
	if (scheduled) return;
	scheduled = true;
	requestAnimationFrame(() => {
		scheduled = false;
		if (dirty) tree();
	});
}

/** Focuses an element by id (mouse hover, programmatic focus). */
export function focusId(id: string): void {
	const l = tree();
	byDefault = false;
	if (nav.focused !== id && l.getNode(id) !== undefined) l.assignFocus(id);
}

/** Handles an app action; returns true when it was used. */
export function handleAction(a: Action): boolean {
	const l = tree();
	byDefault = false;
	const item = items.get(nav.focused);
	if (item?.onKey?.(a) === true) return true;
	switch (a) {
		case 'up':
		case 'down':
		case 'left':
		case 'right':
			l.handleKeyEvent({ direction: a }, { forceFocus: true });
			return true;
		case 'enter':
			if (item === undefined) return false;
			item.el.click();
			return true;
		default:
			return false;
	}
}

/** Svelte action: a focusable element (button, link, card row...). */
export function focusable(el: HTMLElement, opts: FocusableOptions = {}) {
	const id = opts.id ?? nextId('f');
	el.dataset.nav = 'item';
	el.dataset.navId = id;
	if (!el.hasAttribute('tabindex')) el.tabIndex = -1;
	items.set(id, { el, onKey: opts.onKey });
	const hover = () => focusId(id);
	el.addEventListener('pointerenter', hover);
	changed();
	return {
		update(next: FocusableOptions = {}) {
			items.set(id, { el, onKey: next.onKey });
		},
		destroy() {
			el.removeEventListener('pointerenter', hover);
			items.delete(id);
			changed();
		},
	};
}

/** Svelte action: groups focusables along one axis (a menu, a button bar, a page). */
export function focusGroup(el: HTMLElement, opts: { orientation?: Orientation; id?: string } = {}) {
	el.dataset.nav = 'group';
	el.dataset.navId = opts.id ?? nextId('g');
	el.dataset.navOrientation = opts.orientation ?? 'vertical';
	changed();
	return { destroy: changed };
}
