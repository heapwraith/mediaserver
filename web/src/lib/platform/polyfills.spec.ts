import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { installPolyfills } from './polyfills';

describe('polyfills on a browser without the functions', () => {
	const replaceAll = Object.getOwnPropertyDescriptor(String.prototype, 'replaceAll');
	const hasOwn = Object.getOwnPropertyDescriptor(Object, 'hasOwn');

	beforeAll(() => {
		Reflect.deleteProperty(String.prototype, 'replaceAll');
		Reflect.deleteProperty(Object, 'hasOwn');
		installPolyfills();
	});
	afterAll(() => {
		// oxlint-disable-next-line no-extend-native -- puts the native one back
		if (replaceAll) Object.defineProperty(String.prototype, 'replaceAll', replaceAll);
		if (hasOwn) Object.defineProperty(Object, 'hasOwn', hasOwn);
	});

	it.each([
		// What Svelte calls.
		['<!><p><!></p>', '<!>', '<!---->', '<!----><p><!----></p>'],
		['a /* b */ c /* d */', /\/\*.*?\*\//g, '', 'a  c '],
		// Characters that mean something in a regular expression are plain text.
		['a.b.c', '.', '-', 'a-b-c'],
		['(x)+(x)', '(x)', 'y', 'y+y'],
		['nothing', 'z', 'y', 'nothing'],
		['ab', '', '-', '-a-b-'],
	])('replaceAll(%s, %s)', (text, search, replace, want) => {
		expect(text.replaceAll(search, replace)).toBe(want);
	});

	it('replaceAll takes a function', () => {
		expect('a1b2'.replaceAll(/\d/g, (d) => String(Number(d) * 2))).toBe('a2b4');
	});

	it('hasOwn sees only own properties', () => {
		expect(Object.hasOwn({ a: 1 }, 'a')).toBe(true);
		expect(Object.hasOwn({ a: 1 }, 'toString')).toBe(false);
		// An object without a prototype has no hasOwnProperty of its own to call.
		expect(Object.hasOwn({ __proto__: null, a: 1 }, 'a')).toBe(true);
	});
});
