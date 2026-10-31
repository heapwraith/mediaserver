<script lang="ts">
	import { goto } from '$app/navigation';

	import { ApiError, listProfiles, startSession } from '#lib/api/client.ts';
	import { session, setSession } from '#lib/api/session.svelte.ts';
	import type { Profile } from '#lib/api/types.ts';
	import { focusGroup, focusable } from '#lib/platform/nav.svelte.ts';
	import Avatar from '#lib/ui/Avatar.svelte';
	import Button from '#lib/ui/Button.svelte';
	import PinPad from '#lib/ui/PinPad.svelte';

	const MAX_PROFILES = 12; // internal/library/profiles.go

	const profiles = listProfiles();
	/** "Editar perfis": choosing a profile opens its page instead of Início. */
	let editing = $state(false);
	/** The profile asking for its PIN. */
	let locked: Profile | undefined = $state();
	let pinError = $state('');
	let error = $state('');

	const after = (p: Profile) => (editing ? `/perfis/${p.id}` : '/');

	async function choose(p: Profile) {
		error = '';
		if (session.profile?.id === p.id && session.token !== '') {
			await goto(after(p));
			return;
		}
		if (p.has_pin) {
			pinError = '';
			locked = p;
			return;
		}
		try {
			setSession(await startSession(p.id));
			await goto(after(p));
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		}
	}

	async function unlock(p: Profile, pin: string): Promise<boolean> {
		try {
			setSession(await startSession(p.id, pin));
			locked = undefined;
			await goto(after(p));
			return true;
		} catch (e) {
			if (e instanceof ApiError && e.status === 403) pinError = 'PIN incorreto';
			else if (e instanceof ApiError && e.status === 429) pinError = 'Muitas tentativas. Aguarde 30 segundos.';
			else pinError = e instanceof Error ? e.message : String(e);
			return false;
		}
	}
</script>

<svelte:head><title>Quem está assistindo?</title></svelte:head>

<div class="page">
	{#await profiles then list}
		<h1>{list.length === 0 ? 'Crie o primeiro perfil' : editing ? 'Editar perfis' : 'Quem está assistindo?'}</h1>
		<div class="profiles" use:focusGroup={{ id: 'profiles', orientation: 'horizontal' }}>
			{#each list as p (p.id)}
				<button class="profile" use:focusable={{ id: `profile:${p.id}` }} onclick={() => void choose(p)}>
					<Avatar name={p.name} color={p.color} />
					<span class="name">{p.name}</span>
					<span class="hint">{editing ? 'Editar' : p.has_pin ? 'Com PIN' : ''}</span>
				</button>
			{/each}
			{#if list.length < MAX_PROFILES}
				<button class="profile add" use:focusable={{ id: 'profile:new' }} onclick={() => goto('/perfis/novo')}>
					<span class="plus" aria-hidden="true">+</span>
					<span class="name">Adicionar perfil</span>
					<span class="hint"></span>
				</button>
			{/if}
		</div>
		{#if error}<p class="error">{error}</p>{/if}
		{#if list.length > 0}
			<div class="bar" use:focusGroup={{ orientation: 'horizontal' }}>
				<Button id="profiles:edit" selected={editing} onclick={() => (editing = !editing)}>
					{editing ? 'Concluir' : 'Editar perfis'}
				</Button>
			</div>
		{/if}
	{:catch e}
		<p class="error">Não foi possível carregar os perfis: {e instanceof Error ? e.message : e}</p>
	{/await}
</div>

{#if locked}
	{@const p = locked}
	<PinPad
		title={`PIN de ${p.name}`}
		error={pinError}
		onsubmit={(pin: string) => unlock(p, pin)}
		oncancel={() => (locked = undefined)}
	/>
{/if}

<style>
	.page {
		padding-top: 18vh;
		text-align: center;
	}
	h1 {
		margin: 0 0 3rem;
		font-size: 3rem;
		font-weight: 600;
	}
	.profiles {
		display: flex;
		flex-wrap: wrap;
		justify-content: center;
	}
	.profile {
		display: flex;
		flex-direction: column;
		align-items: center;
		width: 11rem;
		margin: 0 1rem 2rem;
		padding: 1rem 0;
		border: 0;
		border-radius: 12px;
		background: transparent;
		color: rgba(255, 255, 255, 0.6);
		font: inherit;
		cursor: pointer;
		transition: transform 200ms ease-out;
	}
	.profile :global(.avatar),
	.plus {
		box-shadow: 0 0 0 3px transparent;
	}
	.profile:global([data-focused]) {
		color: rgba(255, 255, 255, 0.92);
		transform: scale(1.08);
	}
	.profile:global([data-focused]) :global(.avatar),
	.profile:global([data-focused]) .plus {
		box-shadow:
			0 16px 40px rgba(0, 0, 0, 0.6),
			0 0 0 3px rgba(255, 255, 255, 0.92);
	}
	.plus {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 8rem;
		height: 8rem;
		border-radius: 999px;
		background: rgba(255, 255, 255, 0.1);
		font-size: 3.5rem;
		font-weight: 300;
	}
	.name {
		margin-top: 1rem;
		font-size: 1.375rem;
		font-weight: 600;
	}
	.hint {
		min-height: 1.25rem;
		margin-top: 0.25rem;
		font-size: 0.875rem;
		color: rgba(255, 255, 255, 0.4);
	}
	.bar {
		display: flex;
		justify-content: center;
		margin-top: 1rem;
	}
	.error {
		margin: 1rem 0;
		color: #ff8a8a;
		font-size: 1.125rem;
	}
</style>
