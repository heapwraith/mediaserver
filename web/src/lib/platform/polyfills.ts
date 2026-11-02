// Functions Svelte and SvelteKit call and Chromium 76 lacks (AGENTS.md §6). The
// build converts syntax, not functions; scripts/check-build.ts rejects the others.

export function installPolyfills(): void {
	if (!Reflect.has(String.prototype, 'replaceAll')) {
		// oxlint-disable-next-line no-extend-native -- this is what a polyfill does
		Object.defineProperty(String.prototype, 'replaceAll', {
			configurable: true,
			writable: true,
			value(this: string, search: string | RegExp, replace: string) {
				const re =
					typeof search === 'string'
						? new RegExp(search.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'g')
						: search;
				return this.replace(re, replace);
			},
		});
	}
	if (!Reflect.has(Object, 'hasOwn')) {
		Object.defineProperty(Object, 'hasOwn', {
			configurable: true,
			writable: true,
			value: (o: object, key: PropertyKey) => Object.prototype.hasOwnProperty.call(o, key),
		});
	}
}
