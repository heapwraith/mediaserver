import { afterEach, describe, expect, it, vi } from 'vitest';

import { detectCapabilities, supportsASS } from './codecs';

const mp4AndWebm = (t: string) => t === 'video/mp4' || t === 'video/webm';
const someCodecs = (t: string) => /avc1\.64|mp4a\.40\.2|opus/.test(t);
const mp4AndHevc10 = (t: string) => t === 'video/mp4' || t.includes('hvc1.2');

afterEach(() => vi.unstubAllGlobals());

function browser(moduleWorker: boolean, memory = 8) {
	vi.stubGlobal('navigator', { deviceMemory: memory });
	vi.stubGlobal('OffscreenCanvas', function () {});
	vi.stubGlobal(
		'Worker',
		class {
			constructor(_url: string, options: WorkerOptions) {
				if (moduleWorker) void options.type;
			}
			terminate() {}
		},
	);
}

describe('supportsASS', () => {
	it('accepts module workers and rejects the classic-only TV', () => {
		browser(true);
		expect(supportsASS()).toBe(true);
		browser(false);
		expect(supportsASS()).toBe(false);
	});
	it('rejects low memory and missing canvas or WASM', () => {
		browser(true, 2);
		expect(supportsASS()).toBe(false);
		browser(true);
		vi.stubGlobal('OffscreenCanvas', undefined);
		expect(supportsASS()).toBe(false);
		browser(true);
		vi.stubGlobal('WebAssembly', undefined);
		expect(supportsASS()).toBe(false);
	});
	it('rejects worker creation failures', () => {
		browser(true);
		vi.stubGlobal('Worker', function () {
			throw new Error('blocked');
		});
		expect(supportsASS()).toBe(false);
	});
});

describe('detectCapabilities', () => {
	it('asks containers to the element and codecs to MediaSource', () => {
		expect(detectCapabilities(mp4AndWebm, someCodecs)).toEqual([
			'mp4',
			'webm',
			'h264',
			'aac',
			'opus',
		]);
	});

	it('falls back to the element without MediaSource', () => {
		expect(detectCapabilities(mp4AndHevc10, undefined)).toEqual(['mp4', 'hevc:10']);
	});
});
