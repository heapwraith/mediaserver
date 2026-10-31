<script lang="ts">
	import { onMount, tick } from 'svelte';

	import { isAppMessage, keyMessage, type Native } from '../lib/platform/bridge';
	import { isDirection, keyAction } from '../lib/platform/keys';
	import { focusGroup, focusId, focusable, handleAction } from '../lib/platform/nav.svelte';
	import Button from '../lib/ui/Button.svelte';
	import { probe, scan, subnetHosts, subnetsToScan, type Found } from './discovery';
	import {
		DEFAULT_PORT,
		appUrl,
		loadServer,
		parseServer,
		saveServer,
		serverOrigin,
		type Server,
	} from './server';

	let { native }: { native: Native } = $props();

	/** The app must say it started within this time, else this screen comes back. */
	const READY_MS = 15_000;
	/** Keys of the Samsung on-screen keyboard. */
	const IME_DONE = 65376;
	const IME_CANCEL = 65385;

	type Phase = 'starting' | 'form' | 'connecting' | 'app';
	let phase: Phase = $state('starting');
	let saved = loadServer();
	let hostInput = $state(saved?.host ?? '');
	let portInput = $state(String(saved?.port ?? DEFAULT_PORT));
	let found: Found[] = $state([]);
	let scanning = $state(false);
	let progress = $state(0);
	let error = $state('');
	let connectingTo = $state('');
	/** The server whose app is in the frame. */
	let current: Server | undefined = $state();
	let frame: HTMLIFrameElement | undefined = $state();
	/** The server left with "Trocar servidor": Voltar returns to it. */
	let previous: Server | undefined;
	let editing: HTMLInputElement | undefined;
	/** The user pressed a key: a server found later does not take the focus. */
	let touched = false;
	let scanStop: AbortController | undefined;
	let readyTimer: ReturnType<typeof setTimeout> | undefined;

	const label = (s: Server) => `${s.host}:${s.port}`;
	const serverId = (s: Server) => `server:${label(s)}`;

	async function start() {
		// Android reopens this page with #setup for "Trocar servidor".
		if (saved !== undefined && location.hash !== '#setup') {
			phase = 'connecting';
			connectingTo = label(saved);
			if ((await probe(saved, { timeoutMs: 4000 })) !== undefined) {
				open(saved);
				return;
			}
			error = `Não foi possível conectar a ${label(saved)}. O servidor está ligado?`;
		}
		showForm();
	}

	function showForm() {
		phase = 'form';
		void startScan();
	}

	async function startScan() {
		scanStop?.abort();
		const stop = new AbortController();
		scanStop = stop;
		found = [];
		progress = 0;
		scanning = true;
		const typed = parseServer('x', portInput);
		const port = typeof typed === 'string' ? DEFAULT_PORT : typed.port;
		const ip = await native.localIp();
		const hosts = subnetsToScan(ip, saved?.host).flatMap(subnetHosts);
		await scan(hosts, port, {
			signal: stop.signal,
			onFound: (f) => {
				found = [...found, f];
				if (!touched && found.length === 1) void tick().then(() => focusId(serverId(f)));
			},
			onProgress: (done, total) => (progress = Math.round((done * 100) / total)),
		});
		if (scanStop === stop) scanning = false;
	}

	async function connect(s: Server) {
		scanStop?.abort();
		scanning = false;
		phase = 'connecting';
		connectingTo = label(s);
		error = '';
		if ((await probe(s, { timeoutMs: 4000 })) === undefined) {
			error = `Nenhum mediaserver respondeu em ${label(s)}.`;
			showForm();
			return;
		}
		saved = s;
		saveServer(s);
		open(s);
	}

	function connectTyped() {
		const s = parseServer(hostInput, portInput);
		if (typeof s === 'string') error = s;
		else void connect(s);
	}

	/** Shows the server's app: Android navigates to it, Tizen and webOS frame it. */
	function open(s: Server) {
		const url = appUrl(s, native.platform);
		if (native.connect !== undefined) {
			native.connect(url);
			return;
		}
		previous = undefined;
		current = s;
		phase = 'app';
		clearTimeout(readyTimer);
		readyTimer = setTimeout(() => leaveApp('O servidor não abriu o aplicativo.'), READY_MS);
	}

	function leaveApp(message = '') {
		clearTimeout(readyTimer);
		current = undefined;
		error = message;
		showForm();
	}

	function onmessage(e: MessageEvent) {
		if (phase !== 'app' || current === undefined || frame === undefined) return;
		if (e.source !== frame.contentWindow || e.origin !== serverOrigin(current)) return;
		if (!isAppMessage(e.data)) return;
		if (e.data.type === 'ready') {
			clearTimeout(readyTimer);
			frame.focus();
		} else if (e.data.type === 'exit') {
			native.exit();
		} else {
			previous = current;
			leaveApp();
		}
	}

	// Text fields: the TV keyboard opens only on OK, not while the focus
	// passes by (a readonly field does not open it).
	function startEditing(input: HTMLInputElement) {
		if (editing === input) return;
		stopEditing();
		// Blur first (its handler ends an edit), then focus with the field
		// writable: the TV opens its keyboard on that focus.
		input.blur();
		editing = input;
		input.readOnly = false;
		input.focus();
		const end = input.value.length;
		input.setSelectionRange(end, end);
	}

	function stopEditing() {
		const input = editing;
		editing = undefined;
		if (input === undefined) return;
		input.readOnly = true;
		input.blur();
		input.focus({ preventScroll: true });
	}

	function editKey(e: KeyboardEvent): void {
		const code = e.keyCode;
		if (e.key === 'Enter' || code === IME_DONE) {
			e.preventDefault();
			stopEditing();
		} else if (e.key === 'Escape' || code === IME_CANCEL || code === 10009 || code === 461) {
			e.preventDefault();
			stopEditing();
		} else if (e.key === 'ArrowUp' || e.key === 'ArrowDown') {
			e.preventDefault();
			stopEditing();
			const a = keyAction(e);
			if (a !== null) handleAction(a);
		}
		// Anything else edits the text (Backspace included).
	}

	function back() {
		if (phase === 'connecting') return;
		if (previous !== undefined) void connect(previous);
		else native.exit();
	}

	function onkeydown(e: KeyboardEvent) {
		touched = true;
		if (phase === 'app') {
			// The frame lost the focus: hand it the key and the focus back.
			if (frame?.contentWindow != null && current !== undefined) {
				e.preventDefault();
				frame.contentWindow.postMessage(keyMessage(e.key, e.keyCode), serverOrigin(current));
				frame.focus();
			}
			return;
		}
		if (editing !== undefined) {
			editKey(e);
			return;
		}
		const action = keyAction(e);
		if (action === null || e.altKey || e.ctrlKey || e.metaKey) return;
		if (handleAction(action)) e.preventDefault();
		else if (action === 'back') {
			e.preventDefault();
			back();
		} else if (isDirection(action)) e.preventDefault();
	}

	onMount(() => {
		void start();
		return () => {
			scanStop?.abort();
			clearTimeout(readyTimer);
		};
	});
</script>

<svelte:window {onkeydown} {onmessage} />

{#if phase === 'app' && current !== undefined}
	<iframe
		bind:this={frame}
		class="fixed top-0 left-0 h-full w-full border-0 bg-bg"
		src={appUrl(current, native.platform)}
		title="mediaserver"
		allow="autoplay; fullscreen; encrypted-media"
		onload={() => frame?.focus()}
	></iframe>
{:else}
	<main
		class="mx-auto box-border min-h-screen max-w-[64rem] px-[5vw] py-[8vh] text-text-primary"
		use:focusGroup={{ id: 'connect' }}
	>
		<h1 class="m-0 text-[3.5rem] font-700 tracking-tight">mediaserver</h1>
		<p class="mb-12 mt-2 text-[1.75rem] text-text-secondary">Conectar ao servidor</p>

		{#if phase === 'starting' || phase === 'connecting'}
			<p class="text-[2rem]" aria-live="polite">
				{connectingTo === '' ? 'Abrindo…' : `Conectando a ${connectingTo}…`}
			</p>
		{:else}
			{#if error !== ''}
				<p class="mb-8 rounded-card bg-surface-2 px-6 py-4 text-[1.5rem]" role="alert">{error}</p>
			{/if}

			<section class="mb-12">
				<h2 class="mb-4 text-[1.5rem] font-600 text-text-secondary">Servidores nesta rede</h2>
				{#each found as f (serverId(f))}
					<button
						class="server mb-4 block w-full rounded-card border-0 bg-surface-2 px-8 py-5 text-left text-[1.75rem] text-text-primary"
						use:focusable={{ id: serverId(f) }}
						onclick={() => void connect(f)}
					>
						<span class="font-600">{label(f)}</span>
						<span class="ml-4 text-text-secondary">versão {f.version}</span>
					</button>
				{/each}
				<p class="m-0 text-[1.5rem] text-text-tertiary" aria-live="polite">
					{#if scanning}
						Procurando… {progress}%
					{:else if found.length === 0}
						Nenhum servidor encontrado. Digite o endereço abaixo.
					{/if}
				</p>
			</section>

			<section>
				<h2 class="mb-4 text-[1.5rem] font-600 text-text-secondary">Endereço do servidor</h2>
				<div class="flex items-end" use:focusGroup={{ id: 'manual', orientation: 'horizontal' }}>
					<label class="mr-6 block flex-1 text-[1.5rem] text-text-secondary">
						IP ou nome
						<input
							class="field"
							type="text"
							inputmode="url"
							autocomplete="off"
							autocapitalize="off"
							spellcheck="false"
							placeholder="192.168.0.10"
							readonly
							bind:value={hostInput}
							use:focusable={{ id: 'host' }}
							onclick={(e) => startEditing(e.currentTarget)}
							onblur={(e) => {
								if (editing === e.currentTarget) stopEditing();
							}}
						/>
					</label>
					<label class="mr-6 block w-40 text-[1.5rem] text-text-secondary">
						Porta
						<input
							class="field"
							type="text"
							inputmode="numeric"
							autocomplete="off"
							maxlength="5"
							readonly
							bind:value={portInput}
							use:focusable={{ id: 'port' }}
							onclick={(e) => startEditing(e.currentTarget)}
							onblur={(e) => {
								if (editing === e.currentTarget) stopEditing();
							}}
						/>
					</label>
					<Button primary id="connect" onclick={connectTyped}>Conectar</Button>
				</div>
				{#if !scanning}
					<div class="mt-8" use:focusGroup={{ orientation: 'horizontal' }}>
						<Button id="rescan" onclick={startScan}>Procurar de novo</Button>
					</div>
				{/if}
			</section>

			<p class="mt-16 text-[1.375rem] text-text-tertiary">
				O mediaserver precisa estar rodando num computador desta rede.
			</p>
		{/if}
	</main>
{/if}

<style>
	/* Same focus as the app (AGENTS.md §7): scale + shadow + light outline. */
	.server,
	.field {
		transition: transform 200ms ease-out;
	}
	.server:global([data-focused]),
	.field:global([data-focused]) {
		transform: scale(1.08);
		box-shadow:
			0 12px 30px rgba(0, 0, 0, 0.5),
			0 0 0 3px #4c8dff;
	}
	.field {
		display: block;
		width: 100%;
		margin-top: 0.75rem;
		padding: 1rem 1.25rem;
		border: 0;
		border-radius: 12px;
		background: #1e1e26;
		color: rgba(255, 255, 255, 0.92);
		font: inherit;
		font-size: 1.75rem;
		outline: none;
	}
	.field:not([readonly]) {
		background: #282832;
	}
</style>
