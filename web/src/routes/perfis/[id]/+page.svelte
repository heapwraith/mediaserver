<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';

	import { createProfile, deleteProfile, listProfiles, updateProfile } from '#lib/api/client.ts';
	import { clearSession, session, setSession } from '#lib/api/session.svelte.ts';
	import type { Profile, ProfileInput } from '#lib/api/types.ts';
	import { focusGroup, focusable } from '#lib/platform/nav.svelte.ts';
	import Avatar from '#lib/ui/Avatar.svelte';
	import Button from '#lib/ui/Button.svelte';

	// Avatar colors: dark enough for the white initial (WCAG AA).
	const COLORS = ['#4338ca', '#0369a1', '#047857', '#a16207', '#c2410c', '#b91c1c', '#be185d', '#7e22ce'];
	const LANGUAGES = [['', 'Desligadas'], ['pt-BR', 'Português'], ['en', 'Inglês'], ['es', 'Espanhol'], ['ja', 'Japonês']] as const;

	const id = $derived(page.params.id ?? 'novo');
	const isNew = $derived(id === 'novo');

	let name = $state('');
	let color = $state(COLORS[0] ?? '#4338ca');
	let pin = $state('');
	let subtitleLanguage = $state('');
	/** Editing: 'keep' the PIN, 'set' a new one, or 'remove' it. */
	let pinMode: 'keep' | 'set' | 'remove' = $state('keep');
	let current: Profile | undefined = $state();
	let error = $state('');
	let confirmDelete = $state(false);
	let busy = $state(false);

	async function load(profileId: string) {
		if (profileId === 'novo') {
			pinMode = 'set';
			return;
		}
		// Only the profile of this session may change itself.
		if (session.profile?.id !== profileId) {
			await goto('/perfis', { replaceState: true });
			return;
		}
		current = (await listProfiles()).find((p) => p.id === profileId);
		if (current === undefined) {
			await goto('/perfis', { replaceState: true });
			return;
		}
		name = current.name;
		color = current.color;
		subtitleLanguage = current.subtitle_language ?? '';
		pinMode = current.has_pin ? 'keep' : 'set';
	}
	$effect(() => {
		void load(id).catch((e: unknown) => (error = e instanceof Error ? e.message : String(e)));
	});

	const pinValid = $derived(pin === '' || /^\d{4}$/.test(pin));

	async function save() {
		error = '';
		if (name.trim() === '') {
			error = 'Digite um nome.';
			return;
		}
		if (!pinValid) {
			error = 'O PIN precisa ter 4 números.';
			return;
		}
		if (pinMode === 'set' && pin === '' && current?.has_pin === true) {
			error = 'Digite o novo PIN, ou escolha "Remover PIN".';
			return;
		}
		// No pin field keeps the current PIN; "" removes it.
		const input: ProfileInput = { name: name.trim(), color, subtitle_language: subtitleLanguage };
		if (pinMode === 'remove') input.pin = '';
		else if (pinMode === 'set' && pin !== '') input.pin = pin;
		busy = true;
		try {
			if (isNew) {
				setSession(await createProfile(input));
				await goto('/', { replaceState: true });
			} else {
				setSession(await updateProfile(id, input));
				await goto('/perfis', { replaceState: true });
			}
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			busy = false;
		}
	}

	async function remove() {
		if (!confirmDelete) {
			confirmDelete = true;
			return;
		}
		try {
			await deleteProfile(id);
			clearSession();
			await goto('/perfis', { replaceState: true });
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		}
	}
</script>

<svelte:head><title>{isNew ? 'Novo perfil' : 'Editar perfil'}</title></svelte:head>

<div class="page">
	<div class="preview">
		<Avatar name={name || '?'} {color} />
	</div>
	<h1>{isNew ? 'Novo perfil' : 'Editar perfil'}</h1>

	<label class="field">
		<span>Nome</span>
		<input
			type="text"
			maxlength="20"
			autocomplete="off"
			placeholder="Ex.: Ana"
			bind:value={name}
			use:focusable={{ id: 'profile:name' }}
		/>
	</label>

	<div class="field">
		<span>Cor</span>
		<div class="colors" use:focusGroup={{ id: 'profile:colors', orientation: 'horizontal' }}>
			{#each COLORS as c (c)}
				<button
					class="swatch"
					class:selected={c === color}
					style:background={c}
					aria-label={`Cor ${c}`}
					use:focusable={{ id: `profile:color:${c}` }}
					onclick={() => (color = c)}
				></button>
			{/each}
		</div>
	</div>

	<div class="field">
		<span>PIN (opcional)</span>
		{#if pinMode === 'set'}
			<input
				type="password"
				inputmode="numeric"
				maxlength="4"
				autocomplete="off"
				placeholder={current?.has_pin ? 'Novo PIN de 4 números' : '4 números, ou deixe em branco'}
				bind:value={pin}
				use:focusable={{ id: 'profile:pin' }}
			/>
		{:else}
			<p class="note">{pinMode === 'keep' ? 'Este perfil tem PIN.' : 'O PIN será removido ao salvar.'}</p>
		{/if}
		{#if !isNew && current?.has_pin}
			<div class="bar" use:focusGroup={{ orientation: 'horizontal' }}>
				<Button selected={pinMode === 'set'} onclick={() => (pinMode = pinMode === 'set' ? 'keep' : 'set')}>
					Trocar PIN
				</Button>
				<Button selected={pinMode === 'remove'} onclick={() => (pinMode = pinMode === 'remove' ? 'keep' : 'remove')}>
					Remover PIN
				</Button>
			</div>
		{/if}
	</div>

	<div class="field">
		<span>Legenda padrão</span>
		<div class="grid grid-cols-2 sm:grid-cols-3 gap-4" use:focusGroup={{ id: 'profile:subtitles', orientation: 'horizontal' }}>
			{#each LANGUAGES as [value, label] (value)}
				<Button selected={subtitleLanguage === value} onclick={() => (subtitleLanguage = value)}>{label}</Button>
			{/each}
		</div>
	</div>

	{#if error}<p class="error">{error}</p>{/if}

	<div class="bar actions" use:focusGroup={{ orientation: 'horizontal' }}>
		<Button primary id="profile:save" onclick={save}>{busy ? 'Salvando…' : 'Salvar'}</Button>
		<Button onclick={() => goto('/perfis')}>Cancelar</Button>
		{#if !isNew}
			<Button id="profile:delete" onclick={remove}>
				{confirmDelete ? 'Confirmar: excluir perfil e progresso' : 'Excluir perfil'}
			</Button>
		{/if}
	</div>
</div>

<style>
	.page {
		max-width: 48rem;
		margin: 0 auto;
		padding-top: 8vh;
	}
	.preview {
		display: flex;
		justify-content: center;
	}
	h1 {
		margin: 1.5rem 0 2rem;
		text-align: center;
		font-size: 2.5rem;
		font-weight: 600;
	}
	.field {
		display: block;
		margin-bottom: 2rem;
	}
	.field > span {
		display: block;
		margin-bottom: 0.75rem;
		color: rgba(255, 255, 255, 0.6);
		font-size: 1.125rem;
	}
	input {
		width: 100%;
		height: 3.5rem;
		padding: 0 1.5rem;
		border: 0;
		border-radius: 12px;
		background: rgba(255, 255, 255, 0.1);
		color: rgba(255, 255, 255, 0.92);
		font: inherit;
		font-size: 1.375rem;
	}
	input:global([data-focused]) {
		box-shadow: 0 0 0 3px #4c8dff;
	}
	.colors {
		display: flex;
		flex-wrap: wrap;
	}
	.swatch {
		width: 3.5rem;
		height: 3.5rem;
		margin: 0 1rem 0.5rem 0;
		border: 0;
		border-radius: 999px;
		cursor: pointer;
		transition: transform 200ms ease-out;
	}
	.swatch.selected {
		box-shadow: 0 0 0 3px rgba(255, 255, 255, 0.92);
	}
	.swatch:global([data-focused]) {
		transform: scale(1.08);
		box-shadow:
			0 12px 30px rgba(0, 0, 0, 0.5),
			0 0 0 3px #4c8dff;
	}
	.bar {
		display: flex;
		margin-top: 1rem;
	}
	.bar > :global(* + *) {
		margin-left: 1rem;
	}
	.actions {
		margin-top: 2rem;
	}
	.note {
		margin: 0;
		font-size: 1.125rem;
	}
	.error {
		color: #ff8a8a;
		font-size: 1.125rem;
	}
	/* TV: text of at least 24px at 1080p (AGENTS.md §7). */
	:global(html.tv) .field > span,
	:global(html.tv) input,
	:global(html.tv) .note,
	:global(html.tv) .error {
		font-size: 1.5rem;
	}
</style>
