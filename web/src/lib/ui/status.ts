// Formatting of the status page (routes/status).

const units = ['B', 'KB', 'MB', 'GB', 'TB'];

/** 1,5 KB · 50 MB · 10 GB (powers of 1024, Brazilian decimal comma). */
export function bytes(n: number): string {
	let v = n;
	let u = 0;
	while (v >= 1024 && u < units.length - 1) {
		v /= 1024;
		u++;
	}
	const digits = u === 0 || v >= 10 ? 0 : 1;
	return `${v.toLocaleString('pt-BR', { maximumFractionDigits: digits })} ${units[u]}`;
}

/** 5 s · 2 min · 3 h 12 min · 2 d 1 h. */
export function duration(seconds: number): string {
	const s = Math.max(0, Math.floor(seconds));
	if (s < 60) return `${s} s`;
	const m = Math.floor(s / 60);
	if (m < 60) return `${m} min`;
	const h = Math.floor(m / 60);
	if (h < 24) return `${h} h ${m % 60} min`;
	return `${Math.floor(h / 24)} d ${h % 24} h`;
}

const encoders: Record<string, string> = {
	nvenc: 'NVIDIA (NVENC)',
	qsv: 'Intel (Quick Sync)',
	amf: 'AMD (AMF)',
	vaapi: 'VAAPI',
	videotoolbox: 'Apple (VideoToolbox)',
	cpu: 'CPU (sem aceleração)',
	'': 'Indisponível (FFmpeg não encontrado)',
};

/** The video acceleration the server converts with, for people. */
export function encoderName(encoder: string): string {
	return encoders[encoder] ?? encoder;
}
