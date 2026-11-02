<script lang="ts">
	import '@fontsource-variable/archivo/wght.css';
	import '@fontsource/bebas-neue/400.css';
	import 'virtual:uno.css';
	import '../app.css';

	import { onMount } from 'svelte';

	import { afterNavigate, goto } from '$app/navigation';
	import { page } from '$app/state';

	import { admin, finishAdmin } from '#lib/api/admin.svelte.ts';
	import { ApiError, unlockAdmin } from '#lib/api/client.ts';
	import { session } from '#lib/api/session.svelte.ts';
	import favicon from '#lib/assets/favicon.svg';
	import { goBack, trackNavigation } from '#lib/platform/back.ts';
	import { isDirection, keyAction } from '#lib/platform/keys.ts';
	import { focusGroup, focusable, handleAction, setActiveChild } from '#lib/platform/nav.svelte.ts';
	import { connectShell, exitApp, isTV } from '#lib/platform/platform.ts';
	import { settings } from '#lib/platform/settings.svelte.ts';
	import Avatar from '#lib/ui/Avatar.svelte';
	import ExitDialog from '#lib/ui/ExitDialog.svelte';
	import PinPad from '#lib/ui/PinPad.svelte';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();

	let adminError = $state('');
	async function tryAdminPin(pin: string): Promise<boolean> {
		adminError = '';
		try {
			await unlockAdmin(pin);
			finishAdmin(true);
			return true;
		} catch (e) {
			adminError =
				e instanceof ApiError && e.code === 'pin_locked'
					? 'Muitas tentativas. Espere um pouco.'
					: 'PIN errado.';
			return false;
		}
	}

	// The rail (ADR 013): only what is there to watch. Revisar lives in Configurações.
	const menu = [
		{ href: '/busca', label: 'Buscar', icon: 'busca' },
		{ href: '/', label: 'Início', icon: 'inicio' },
		{ href: '/filmes', label: 'Filmes', icon: 'filmes' },
		{ href: '/series', label: 'Séries', icon: 'series' },
		{ href: '/animes', label: 'Animes', icon: 'animes' },
	] as const;
	const top = new Set<string>([...menu.map((m) => m.href), '/configuracoes', '/revisar']);
	const active = (href: string) =>
		href === '/' ? page.url.pathname === '/' : page.url.pathname.startsWith(href);
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

	// Entering the rail lands on the current page, not on its first item.
	$effect(() => {
		const here = menu.find((m) => active(m.href));
		setActiveChild('menu', here === undefined ? 'menu:/configuracoes' : `menu:${here.href}`);
	});

	afterNavigate(trackNavigation);

	// TV apps: the layout scales with the screen (app.css) and Voltar on
	// Início asks whether to leave.
	let askingExit = $state(false);
	const atHome = isTV ? () => (askingExit = true) : undefined;
	onMount(() => {
		document.documentElement.classList.toggle('tv', isTV);
		connectShell();
	});

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
		} else if (askingExit) {
			e.preventDefault(); // the dialog keeps every key
		} else if (action === 'back') {
			e.preventDefault();
			goBack(top, atHome);
		} else if (action === 'yellow') {
			e.preventDefault();
			void goto('/busca');
		} else if (action === 'blue') {
			e.preventDefault();
			void goto('/');
		} else if (isDirection(action)) {
			e.preventDefault(); // no native scrolling: the focus drives the page
		}
	}
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>Telão</title>
</svelte:head>

<svelte:window {onkeydown} />

<div class="app" class:tv={isTV} use:focusGroup={{ id: 'app', orientation: 'horizontal' }}>
	{#if !fullscreen && !profilePages}
		<nav class="rail" aria-label="Menu" use:focusGroup={{ id: 'menu' }}>
			<a class="logo" href="/" tabindex="-1" aria-label="Telão: início">
				<svg viewBox="0 0 64 64" aria-hidden="true"
					><rect width="64" height="64" rx="16" fill="var(--accent)" /><path
						d="M15 23h34v20H15z"
						fill="none"
						stroke="#fff"
						stroke-width="4"
						stroke-linejoin="round"
					/><path d="M24 49h16" stroke="#fff" stroke-width="4" stroke-linecap="round" /></svg
				>
				<span>Telão</span>
			</a>
			{#each menu as m (m.href)}
				<a href={m.href} class:active={active(m.href)} use:focusable={{ id: `menu:${m.href}` }}>
					<svg viewBox="0 0 24 24" aria-hidden="true"><use href={`#rail-${m.icon}`} /></svg>
					<span>{m.label}</span>
				</a>
			{/each}
			<div class="foot">
				<a
					href="/configuracoes"
					class:active={active('/configuracoes') || active('/revisar') || active('/status')}
					use:focusable={{ id: 'menu:/configuracoes' }}
				>
					<svg viewBox="0 0 24 24" aria-hidden="true"><use href="#rail-config" /></svg>
					<span>Configurações</span>
				</a>
				{#if session.profile}
					<a
						href="/perfis"
						class="who"
						aria-label={`Perfil ${session.profile.name}: trocar perfil`}
						use:focusable={{ id: 'menu:/perfis' }}
					>
						<span class="avatar"><Avatar name={session.profile.name} color={session.profile.color} size={2.75} /></span>
						<span>{session.profile.name}</span>
					</a>
				{/if}
			</div>
		</nav>
	{/if}
	<main class:fullscreen class:bare={profilePages} use:focusGroup={{ id: 'main' }}>
		{#if allowed}{@render children()}{/if}
	</main>
</div>

<!-- Rail icons: one stroke family, drawn for this app. -->
<svg width="0" height="0" style="position:absolute" aria-hidden="true">
	<defs>
		<symbol id="rail-busca" viewBox="0 0 24 24"
			><circle cx="10.5" cy="10.5" r="6.5" fill="none" stroke="currentColor" stroke-width="2.2" /><path
				d="m15.5 15.5 5 5"
				stroke="currentColor"
				stroke-width="2.2"
				stroke-linecap="round"
			/></symbol
		>
		<symbol id="rail-inicio" viewBox="0 0 24 24"
			><path
				d="M3.5 10.5 12 3.5l8.5 7V20a1 1 0 0 1-1 1H15v-6.5H9V21H4.5a1 1 0 0 1-1-1Z"
				fill="none"
				stroke="currentColor"
				stroke-width="2.2"
				stroke-linejoin="round"
			/></symbol
		>
		<symbol id="rail-filmes" viewBox="0 0 24 24"
			><rect x="3" y="4.5" width="18" height="15" rx="2" fill="none" stroke="currentColor" stroke-width="2.2" /><path
				d="M7.5 4.5v15M16.5 4.5v15M3 9.5h4.5M3 14.5h4.5M16.5 9.5H21M16.5 14.5H21"
				stroke="currentColor"
				stroke-width="2.2"
			/></symbol
		>
		<symbol id="rail-series" viewBox="0 0 24 24"
			><rect x="2.5" y="6" width="19" height="13" rx="2" fill="none" stroke="currentColor" stroke-width="2.2" /><path
				d="m8 2.5 4 3.5 4-3.5"
				fill="none"
				stroke="currentColor"
				stroke-width="2.2"
				stroke-linecap="round"
				stroke-linejoin="round"
			/></symbol
		>
		<symbol id="rail-animes" viewBox="0 0 24 24"
			><path
				d="m12 3 2.6 5.6 6.1.7-4.5 4.2 1.2 6L12 16.6 6.6 19.5l1.2-6L3.3 9.3l6.1-.7Z"
				fill="none"
				stroke="currentColor"
				stroke-width="2.2"
				stroke-linejoin="round"
			/></symbol
		>
		<symbol id="rail-config" viewBox="0 0 24 24"
			><circle cx="12" cy="12" r="3.2" fill="none" stroke="currentColor" stroke-width="2.2" /><path
				d="M12 2.8v2.6M12 18.6v2.6M2.8 12h2.6M18.6 12h2.6M5.5 5.5l1.8 1.8M16.7 16.7l1.8 1.8M5.5 18.5l1.8-1.8M16.7 7.3l1.8-1.8"
				stroke="currentColor"
				stroke-width="2.2"
				stroke-linecap="round"
			/></symbol
		>
	</defs>
</svg>

{#if admin.prompt}
	<PinPad
		id="admin-pin"
		title="PIN de administrador"
		error={adminError}
		onsubmit={tryAdminPin}
		oncancel={() => finishAdmin(false)}
	/>
{/if}

{#if askingExit}
	<ExitDialog onexit={exitApp} oncancel={() => (askingExit = false)} />
{/if}

<style>
	.app {
		min-height: 100vh;
	}
	/* The rail: folded to icons; it opens with labels while it holds the focus. */
	.rail {
		position: fixed;
		top: 0;
		left: 0;
		bottom: 0;
		z-index: 20;
		width: var(--rail);
		padding: 2.5rem 0 2.5rem 2.125rem;
		background: linear-gradient(90deg, rgba(0, 0, 0, 0.96) 0%, rgba(0, 0, 0, 0.7) 70%, rgba(0, 0, 0, 0) 100%);
		transition: width 220ms cubic-bezier(0.2, 0.8, 0.2, 1);
	}
	.rail:focus-within {
		width: 25rem;
		background: linear-gradient(90deg, #000 0%, rgba(0, 0, 0, 0.96) 75%, rgba(0, 0, 0, 0) 100%);
	}
	.rail a {
		display: flex;
		align-items: center;
		width: 4rem;
		height: 4.25rem;
		margin-bottom: 0.625rem;
		border-radius: var(--radius);
		color: var(--text-2);
		text-decoration: none;
		white-space: nowrap;
		overflow: hidden;
		transition:
			width 220ms cubic-bezier(0.2, 0.8, 0.2, 1),
			background-color 180ms ease-out,
			color 180ms ease-out;
	}
	.rail a > svg {
		flex: none;
		width: 4rem;
		height: 1.875rem;
	}
	.rail a > span {
		margin-left: 0.25rem;
		font-size: 1.625rem;
		font-weight: 700;
		opacity: 0;
		transition: opacity 200ms ease-out;
	}
	.rail a.active {
		color: var(--text);
	}
	.rail a.active > svg {
		color: var(--accent);
	}
	.rail:focus-within a {
		width: 18.75rem;
	}
	.rail:focus-within a > span {
		opacity: 1;
	}
	.rail a:global([data-focused]) {
		background: var(--text);
		color: #000;
	}
	.rail a:global([data-focused]) > svg {
		color: #000;
	}
	.rail a.logo {
		height: 4rem;
		margin-bottom: 3.75rem;
		color: var(--text);
	}
	.rail a.logo > svg {
		width: 4rem;
		height: 4rem;
	}
	.rail a.logo > span {
		margin-left: 1.125rem;
		font-family: var(--font-display);
		font-size: 3.125rem;
		font-weight: 400;
		letter-spacing: 0.02em;
	}
	.foot {
		position: absolute;
		left: 2.125rem;
		bottom: 2.5rem;
	}
	.rail a.who {
		margin-bottom: 0;
	}
	.avatar {
		display: flex;
		flex: none;
		justify-content: center;
		width: 4rem;
	}
	main {
		/* TV safe area (AGENTS.md §7) plus room for the folded rail. */
		padding: 0 5vw 5vh var(--content-left);
	}
	main.fullscreen,
	main.bare {
		padding: 0;
	}
	main.bare {
		padding: 0 5vw 5vh;
	}
</style>
