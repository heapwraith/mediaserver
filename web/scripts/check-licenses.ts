// Checks the licenses of the packages that end up in what we ship (run by `make check`).
// Only production dependencies are inspected; build tools (Vite, UnoCSS, Lightning CSS...)
// are not distributed. The project is GPL-3.0-or-later (docs/adr/014-licenca.md), so every
// runtime dependency must be compatible with it: permissive, LGPL, GPL-3 or OFL fonts.
// Anything else (GPL-2.0-only, AGPL, SSPL, CC-NC, unknown...) fails the check.
// Usage: node scripts/check-licenses.ts

import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const allowed = [
	/^MIT/,
	/^ISC$/,
	/^BSD-[23]-Clause$/,
	/^0BSD$/,
	/^Apache-2\.0$/,
	/^OFL-1\.1$/,
	/^CC0-1\.0$/,
	/^Unlicense$/,
	/^Zlib$/,
	/^BlueOak-1\.0\.0$/,
	/^LGPL-2\.1-or-later/,
	/^LGPL-3\.0/,
	/^GPL-3\.0/,
];

// Packages whose SPDX expression is compound or unusual, each with the reason it was accepted.
const reviewed: Record<string, string> = {
	jassub:
		'LGPL-2.1+ with MIT/ISC/FTL parts (WebAssembly build of libass); see THIRD_PARTY_NOTICES.md',
};

type Node = { version?: string; dependencies?: Record<string, Node> };

function walk(tree: Record<string, Node>, out: Map<string, string>): void {
	for (const [name, node] of Object.entries(tree)) {
		if (node.version) out.set(name, node.version);
		if (node.dependencies) walk(node.dependencies, out);
	}
}

const raw = execFileSync('npm', ['ls', '--omit=dev', '--all', '--json'], {
	encoding: 'utf8',
	maxBuffer: 64 * 1024 * 1024,
	// On Windows npm is npm.cmd, which Node only runs through a shell (fixed arguments).
	shell: process.platform === 'win32',
});
const deps = new Map<string, string>();
const tree: Node = JSON.parse(raw);
walk(tree.dependencies ?? {}, deps);

const bad: string[] = [];
for (const name of [...deps.keys()].sort()) {
	const pkg: { license?: string | { type?: string } } = JSON.parse(
		readFileSync(join('node_modules', name, 'package.json'), 'utf8'),
	);
	const license = typeof pkg.license === 'string' ? pkg.license : (pkg.license?.type ?? 'UNKNOWN');
	if (name in reviewed) continue;
	if (!allowed.some((re) => re.test(license))) bad.push(`${name}@${deps.get(name)}: ${license}`);
}

if (bad.length > 0) {
	console.error(`Licenses not allowed (see docs/conformidade.md):\n  ${bad.join('\n  ')}`);
	process.exit(1);
}
console.log(`licenses ok: ${deps.size} production packages`);
