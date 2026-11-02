import { describe, expect, it } from 'vitest';

import { collectionCard, collectionLabel, finishedCard, watchedLast, type CardItem } from './cards';

const card = (key: string): CardItem => ({ key, imageId: key, title: key, href: `/filme/${key}` });

describe('watchedLast', () => {
	it('moves the finished titles to the end, in order, with a check mark', () => {
		const out = watchedLast([card('a'), card('b'), card('c'), card('d')], new Set(['a', 'c']));
		expect(out.map((c) => c.key)).toEqual(['b', 'd', 'a', 'c']);
		expect(out.map((c) => c.watched === true)).toEqual([false, false, true, true]);
	});
});

describe('finishedCard', () => {
	it('opens the details page', () => {
		expect(finishedCard({ kind: 'movie', id: 'm', title: 'M', at: 1 })).toMatchObject({
			href: '/filme/m',
			subtitle: 'Filme',
			watched: true,
		});
		expect(finishedCard({ kind: 'anime', id: 's', title: 'S', at: 1 }).href).toBe('/serie/s');
	});
});

describe('collectionLabel', () => {
	it('does not repeat the word', () => {
		expect(collectionLabel('Star Wars')).toBe('Coleção: Star Wars');
		expect(collectionLabel('Coleção Star Wars')).toBe('Coleção Star Wars');
		expect(collectionLabel('Star Wars: A Coleção')).toBe('Star Wars: A Coleção');
	});
});

describe('collectionCard', () => {
	it('opens the collection, with its oldest movie as the image', () => {
		expect(collectionCard({ id: 7, name: 'Star Wars', count: 3, image_id: 'm1' })).toMatchObject({
			href: '/colecao/7',
			imageId: 'm1',
			title: 'Star Wars',
			subtitle: '3 filmes',
		});
	});
});
