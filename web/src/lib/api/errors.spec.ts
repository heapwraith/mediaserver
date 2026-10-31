import { describe, expect, it } from 'vitest';

import { ApiError } from './client';
import { describeError } from './errors';

describe('describeError', () => {
	it.each([
		[new ApiError(0, 'servidor indisponível'), 'Servidor indisponível', 'retry'],
		[new ApiError(410, 'media file missing', 'file_missing'), 'Arquivo não encontrado', 'scan'],
		[new ApiError(503, 'ffmpeg not available', 'no_ffmpeg'), 'Conversão indisponível', undefined],
		[new ApiError(503, 'too many transcodes running', 'busy'), 'Servidor ocupado', 'retry'],
		[new ApiError(503, 'server shutting down', 'shutting_down'), 'Servidor reiniciando', 'retry'],
		[new ApiError(404, 'not found', 'not_found'), 'Não encontrado', undefined],
		[new ApiError(500, 'internal error', 'internal'), 'Algo deu errado', 'retry'],
		['unsupported', 'Formato não suportado', undefined],
		[new Error('boom'), 'Algo deu errado', 'retry'],
	])('%o → %s', (e, title, action) => {
		const p = describeError(e);
		expect(p.title).toBe(title);
		expect(p.hint).not.toBe('');
		expect(p.action).toBe(action);
	});
});
