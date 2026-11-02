// The TV apps' local page (docs/adr/008): finds the server and shows its app.
// Built by vite.tv.config.ts into a single web/build-tv/index.html.

import 'virtual:uno.css';
import '../app.css';

import archivo from '@fontsource-variable/archivo/files/archivo-latin-wght-normal.woff2';
import bebas from '@fontsource/bebas-neue/files/bebas-neue-latin-400-normal.woff2';
import { mount } from 'svelte';

import { detectNative } from '../lib/platform/native';
import Connect from './Connect.svelte';

// Only the latin subset (Portuguese fits in it), inlined: a page opened from
// file:// cannot always load fonts from separate files.
if (typeof FontFace === 'function') {
	const faces = [
		new FontFace('Archivo Variable', `url(${archivo}) format('woff2')`, { weight: '100 900' }),
		new FontFace('Bebas Neue', `url(${bebas}) format('woff2')`),
	];
	for (const face of faces) {
		document.fonts.add(face);
		void face.load().catch(() => undefined);
	}
}

const native = detectNative(window);
native.registerKeys();
mount(Connect, { target: document.body, props: { native } });
