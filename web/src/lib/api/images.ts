export type ImageVariant = 'card' | 'backdrop' | 'blur';

export const imageWidths = [480, 960, 1920] as const;

/** URL of a generated 16:9 image (any movie, series, season, episode or media id). */
export function imageURL(id: string, variant: ImageVariant = 'card', w = 960): string {
	return `/api/images/${id}?variant=${variant}&w=${w}`;
}

/** srcset with every generated width; the browser picks by the rendered size. */
export function imageSrcset(id: string, variant: ImageVariant = 'card'): string {
	return imageWidths.map((w) => `${imageURL(id, variant, w)} ${w}w`).join(', ');
}
