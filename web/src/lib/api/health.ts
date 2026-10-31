export interface Health {
	status: string;
	version: string;
	db: string;
}

function isHealth(v: unknown): v is Health {
	return (
		typeof v === 'object' &&
		v !== null &&
		'status' in v &&
		typeof v.status === 'string' &&
		'version' in v &&
		typeof v.version === 'string' &&
		'db' in v &&
		typeof v.db === 'string'
	);
}

/** Reads GET /api/health. The 503 "degraded" answer is still a valid Health. */
export async function fetchHealth(fetchFn: typeof fetch = fetch): Promise<Health> {
	const res = await fetchFn('/api/health', { headers: { Accept: 'application/json' } });
	const body: unknown = await res.json();
	if (!isHealth(body)) {
		throw new Error(`unexpected /api/health response (HTTP ${res.status})`);
	}
	return body;
}
