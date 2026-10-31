import { describe, expect, it } from 'vitest';

import { appUrl, parseServer, serverOrigin } from './server';

describe('parseServer', () => {
	it.each([
		['192.168.0.10', '8080', { host: '192.168.0.10', port: 8080 }],
		[' 192.168.0.10 ', '', { host: '192.168.0.10', port: 8080 }],
		['192.168.0.10:8096', '8080', { host: '192.168.0.10', port: 8096 }],
		['http://MeuPC.local:9000/', '', { host: 'meupc.local', port: 9000 }],
		['nas', '80', { host: 'nas', port: 80 }],
	])('%s + %s', (host, port, want) => {
		expect(parseServer(host, port)).toEqual(want);
	});

	it.each([
		['', '8080', 'Digite o endereço do servidor.'],
		['192.168.0.10/../x', '8080', 'Endereço inválido.'],
		['meu pc', '8080', 'Endereço inválido.'],
		['192.168.0.10', '0', 'Porta inválida.'],
		['192.168.0.10', '70000', 'Porta inválida.'],
		['192.168.0.10', 'abc', 'Porta inválida.'],
	])('%s + %s → error', (host, port, want) => {
		expect(parseServer(host, port)).toBe(want);
	});
});

describe('urls', () => {
	const s = { host: '192.168.0.10', port: 8080 };
	it('opens the app with the platform', () => {
		expect(serverOrigin(s)).toBe('http://192.168.0.10:8080');
		expect(appUrl(s, 'tizen')).toBe('http://192.168.0.10:8080/?tv=tizen');
		expect(appUrl(s, 'browser')).toBe('http://192.168.0.10:8080/');
	});
	it('matches the origin the browser reports for port 80', () => {
		expect(serverOrigin({ host: 'nas', port: 80 })).toBe('http://nas');
	});
});
