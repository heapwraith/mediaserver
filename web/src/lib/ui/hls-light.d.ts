// hls.js ships no types for its "light" build (same API, fewer features).
declare module 'hls.js/light' {
	export * from 'hls.js';
	export { default } from 'hls.js';
}
