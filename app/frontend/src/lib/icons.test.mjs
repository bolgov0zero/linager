// Run with: node --test src/lib/icons.test.mjs
// An icon that the set does not have is drawn as an empty space (the button keeps its room): the terminal button "Snippets" had
// `file-code`, which only the mockup set knew. Every icon named in the sources must be in icons.ts.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../', import.meta.url));
const walk = (dir) => readdirSync(dir).flatMap((n) => {
  const p = join(dir, n);
  return statSync(p).isDirectory() ? walk(p) : [p];
});

test('every icon named in the sources exists in the icon set', () => {
  const set = new Set([...readFileSync(join(root, 'lib/icons.ts'), 'utf8').matchAll(/^\s*"([a-z0-9-]+)"\s*:/gm)].map((m) => m[1]));
  assert.ok(set.size > 50, 'the set was read');
  const missing = [];
  for (const f of walk(root).filter((p) => /\.(svelte|ts)$/.test(p) && !/icons\.ts$|\.test\./.test(p))) {
    const src = readFileSync(f, 'utf8');
    for (const m of src.matchAll(/(?:<Icon[^>]*\bname=|\bicon=|\bicon:\s*|\baltIcon=)(?:\{?\s*)['"]([a-z0-9-]+)['"]/g)) {
      if (!set.has(m[1])) missing.push(`${f.slice(root.length)}: ${m[1]}`);
    }
  }
  assert.deepEqual(missing, []);
});
