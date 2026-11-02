// Errors for people: what happened, what to do, and the button that helps.
// The codes come from the API (internal/api: errorBody.code).

import { ApiError } from './client';

export interface Problem {
	title: string;
	hint: string;
	/** retry: load again · scan: the library is out of date. */
	action?: 'retry' | 'scan';
}

/** The client gave up playing a file (direct and converted). */
export const UNSUPPORTED = 'unsupported';

const problems: Record<string, Problem> = {
	offline: {
		title: 'Servidor indisponível',
		hint: 'Confira se o mediaserver está ligado e se este aparelho está na mesma rede.',
		action: 'retry',
	},
	file_missing: {
		title: 'Arquivo não encontrado',
		hint: 'O vídeo foi movido ou apagado da pasta de mídias. Escaneie a biblioteca para atualizá-la.',
		action: 'scan',
	},
	no_ffmpeg: {
		title: 'Conversão indisponível',
		hint: 'Este aparelho não toca o arquivo como ele está, e o servidor não encontrou o FFmpeg para convertê-lo. Instale o FFmpeg no computador do servidor (veja o README).',
	},
	busy: {
		title: 'Servidor ocupado',
		hint: 'Outras conversões estão em andamento. Tente de novo em instantes ou aumente MAX_TRANSCODES.',
		action: 'retry',
	},
	shutting_down: {
		title: 'Servidor reiniciando',
		hint: 'Tente de novo em instantes.',
		action: 'retry',
	},
	not_found: {
		title: 'Não encontrado',
		hint: 'Este item não existe mais na biblioteca.',
	},
	unavailable: {
		title: 'Recurso indisponível',
		hint: 'O servidor iniciou sem esta parte. Veja o log do servidor.',
	},
	tmdb_key: {
		title: 'O TMDB recusou a chave',
		hint: 'Confira a chave em Configurações e salve de novo.',
	},
	tmdb_unavailable: {
		title: 'Sem resposta do TMDB',
		hint: 'Confira a internet do servidor e tente de novo.',
		action: 'retry',
	},
	[UNSUPPORTED]: {
		title: 'Formato não suportado',
		hint: 'Este aparelho não conseguiu tocar o vídeo, nem convertido. Veja "Este aparelho" em Configurações.',
	},
};

const fallback: Problem = {
	title: 'Algo deu errado',
	hint: 'Tente de novo. Se continuar, veja o log do servidor.',
	action: 'retry',
};

/** The problem of an error (ApiError, a code string or anything thrown). */
export function describeError(e: unknown): Problem {
	const code = typeof e === 'string' ? e : e instanceof ApiError ? e.code : '';
	return problems[code] ?? fallback;
}
