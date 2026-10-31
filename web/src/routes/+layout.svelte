<script lang="ts">
	import '@fontsource-variable/inter/index.css';
	import 'virtual:uno.css';
	import '../app.css';

	import { afterNavigate, goto } from '$app/navigation';
	import { page } from '$app/state';

	import { session } from '#lib/api/session.svelte.ts';
	import favicon from '#lib/assets/favicon.svg';
	import { goBack, trackNavigation } from '#lib/platform/back.ts';
	import { isTV } from '#lib/platform/device.ts';
	import { isDirection, keyAction } from '#lib/platform/keys.ts';
	import { focusGroup, focusable, handleAction } from '#lib/platform/nav.svelte.ts';
	import { settings } from '#lib/platform/settings.svelte.ts';
	import Avatar from '#lib/ui/Avatar.svelte';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();

	const menu = [
		{ href: '/', label: 'Início' },
		{ href: '/filmes', label: 'Filmes' },
		{ href: '/series', label: 'Séries' },
		{ href: '/animes', label: 'Animes' },
		{ href: '/busca', label: 'Buscar' },
		{ href: '/revisar', label: 'Revisar' },
		{ href: '/configuracoes', label: 'Configurações' },
	];
	const top = new Set(menu.map((m) => m.href));
	const fullscreen = $derived(page.url.pathname.startsWith('/assistir/'));
	// "Quem está assistindo?" and the profile form need no session.
	const profilePages = $derived(page.url.pathname.startsWith('/perfis'));
	const allowed = $derived(session.token !== '' || profilePages);

	// No profile chosen (or its session ended): ask who is watching.
	$effect(() => {
		if (!allowed) void goto('/perfis', { replaceState: true });
	});

	$effect(() => {
		document.documentElement.classList.toggle('reduce-motion', settings.reduceMotion);
	});

	afterNavigate(trackNavigation);

	function onkeydown(e: KeyboardEvent) {
		const action = keyAction(e);
		if (action === null || e.altKey || e.ctrlKey || e.metaKey) return;
		const t = e.target;
		const typing = t instanceof HTMLInputElement || t instanceof HTMLTextAreaElement;
		// In a text field, keys edit the text; only Esc and up/down leave it.
		if (typing && (e.key === 'Backspace' || e.key === ' ' || action === 'left' || action === 'right')) {
			return;
		}
		if (handleAction(action)) {
			e.preventDefault();
		} else if (action === 'back') {
			e.preventDefault();
			goBack(top);
		} else if (isDirection(action)) {
			e.preventDefault(); // no native scrolling: the focus drives the page
		}
	}
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>mediaserver</title>
</svelte:head>

<svelte:window {onkeydown} />

<div class="app" class:tv={isTV} use:focusGroup={{ id: 'app' }}>
	{#if !fullscreen && !profilePages}
		<nav class="menu" use:focusGroup={{ id: 'menu', orientation: 'horizontal' }}>
			{#each menu as m (m.href)}
				<a
					href={m.href}
					class:active={m.href === '/' ? page.url.pathname === '/' : page.url.pathname.startsWith(m.href)}
					use:focusable={{ id: `menu:${m.href}` }}>{m.label}</a
				>
			{/each}
			{#if session.profile}
				<a
					href="/perfis"
					class="who"
					title={`${session.profile.name} · trocar perfil`}
					aria-label={`Perfil ${session.profile.name}: trocar perfil`}
					use:focusable={{ id: 'menu:/perfis' }}
				>
					<Avatar name={session.profile.name} color={session.profile.color} size={2.25} />
				</a>
			{/if}
		</nav>
	{/if}
	<main class:fullscreen use:focusGroup={{ id: 'main' }}>
		{#if allowed}{@render children()}{/if}
	</main>
</div>

<style>
	.app {
		min-height: 100vh;
	}
	.menu {
		display: flex;
		justify-content: center;
		padding: 2.5vh 5vw 1rem;
	}
	.menu a {
		margin: 0 0.25rem;
		padding: 0.6rem 1.25rem;
		border-radius: 999px;
		color: rgba(255, 255, 255, 0.6);
		font-size: 1.125rem;
		font-weight: 500;
		text-decoration: none;
		transition: transform 200ms ease-out;
	}
	.menu a.active {
		color: rgba(255, 255, 255, 0.92);
		background: rgba(255, 255, 255, 0.1);
	}
	.menu a.who {
		display: flex;
		align-items: center;
		padding: 0.25rem;
		margin-left: 1rem;
	}
	.menu a:global([data-focused]) {
		color: #0b0b0f;
		background: rgba(255, 255, 255, 0.92);
		transform: scale(1.08);
	}
	main {
		/* TV safe area: 5% from the edges (AGENTS.md §7). */
		padding: 0 5vw 5vh;
	}
	main.fullscreen {
		padding: 0;
	}
</style>
