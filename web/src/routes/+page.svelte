<script lang="ts">
	import { fetchHealth } from '#lib';

	// Sprint 0: only proves that the binary serves the embedded frontend.
	const health = fetchHealth();
</script>

<svelte:head>
	<title>mediaserver</title>
</svelte:head>

<main class="min-h-screen flex flex-col items-center justify-center bg-bg text-text-primary font-sans">
	<h1 class="text-5xl font-semibold m-0">mediaserver — ok</h1>
	<p class="mt-4 text-xl text-text-secondary" data-testid="version">
		{#await health}
			conectando…
		{:then h}
			versão {h.version} · banco {h.db}
		{:catch}
			servidor indisponível
		{/await}
	</p>
</main>

<style>
	:global(html, body) {
		margin: 0;
		background: #0b0b0f;
	}
</style>
