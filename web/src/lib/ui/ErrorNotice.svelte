<script lang="ts">
	import { startScan } from '../api/client';
	import { describeError } from '../api/errors';
	import { focusGroup } from '../platform/nav.svelte';
	import Button from './Button.svelte';

	// An error said for people (lib/api/errors.ts), with the button that helps.
	let { error, onretry }: { error: unknown; onretry?: () => void } = $props();

	const problem = $derived(describeError(error));
	let scanning = $state(false);

	async function scan() {
		await startScan().catch(() => undefined);
		scanning = true;
	}

	function retry() {
		if (onretry !== undefined) onretry();
		else location.reload();
	}
</script>

<div class="notice" role="alert">
	<h2>{problem.title}</h2>
	<p>{problem.hint}</p>
	{#if scanning}<p class="note">Escaneando a biblioteca. Volte em instantes.</p>{/if}
	{#if problem.action !== undefined}
		<div class="bar" use:focusGroup={{ orientation: 'horizontal' }}>
			{#if problem.action === 'scan' && !scanning}
				<Button primary id="error:scan" onclick={scan}>Escanear biblioteca</Button>
			{/if}
			<Button primary={problem.action === 'retry'} id="error:retry" onclick={retry}>Tentar de novo</Button>
		</div>
	{/if}
</div>

<style>
	.notice {
		max-width: 48rem;
		margin: 4rem auto;
		padding: 2.5rem 3rem;
		border-radius: 12px;
		background: #15151b;
	}
	h2 {
		margin: 0 0 0.75rem;
		font-size: 2rem;
		font-weight: 600;
	}
	p {
		margin: 0 0 1.5rem;
		color: rgba(255, 255, 255, 0.6);
		font-size: 1.25rem;
		line-height: 1.5;
	}
	.note {
		color: rgba(255, 255, 255, 0.92);
	}
	.bar {
		display: flex;
	}
	.bar > :global(* + *) {
		margin-left: 1rem;
	}
	/* TV: text of at least 24px at 1080p (AGENTS.md §7). */
	:global(html.tv) p {
		font-size: 1.5rem;
	}
</style>
