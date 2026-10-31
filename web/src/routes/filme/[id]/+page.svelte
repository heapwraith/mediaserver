<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';

	import { getMovie } from '#lib/api/client.ts';
	import Button from '#lib/ui/Button.svelte';
	import Hero from '#lib/ui/Hero.svelte';
	import Row from '#lib/ui/Row.svelte';
	import { extraCard } from '#lib/ui/cards.ts';
	import { movieHero } from '#lib/ui/hero.ts';

	const data = $derived(getMovie(page.params.id ?? '').then(movieHero));
</script>

{#await data then h}
	<Hero title={h.title} meta={h.meta} imageId={h.imageId} color={h.color} trailerSrc={h.trailerSrc}>
		{#snippet actions()}
			<Button primary id="movie:play" onclick={() => goto(h.play ?? '/')}>▶ Assistir</Button>
			{#if h.trailer}
				{@const t = h.trailer}
				<Button onclick={() => goto(extraCard(t).href)}>Assistir trailer</Button>
			{/if}
		{/snippet}
	</Hero>
	<Row id="extras" title="Extras" items={h.extras.map(extraCard)} />
{:catch e}
	<p class="error">Filme não encontrado ({e instanceof Error ? e.message : e}).</p>
{/await}

<style>
	.error {
		margin-top: 20vh;
		text-align: center;
		font-size: 1.25rem;
		color: rgba(255, 255, 255, 0.6);
	}
</style>
