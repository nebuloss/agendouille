import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative, resolve } from 'node:path';

// src/app and src/admin may import src/shared, never each other, and
// src/shared imports neither (PLAN.md 9.5). The admin bundle must not leak
// into the family app's, which the public listener serves, and the reverse
// would tie the admin UI to the app's pages.
const SRC = resolve(import.meta.dirname);
const IMPORT = /(?:^|\n)\s*(?:import|export)\b[^'"]*?from\s*['"]([^'"]+)['"]|import\(\s*['"]([^'"]+)['"]\s*\)/g;

function files(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) return files(path);
    return path.endsWith('.ts') ? [path] : [];
  });
}

/** The src/ top directory an import specifier lands in, or null outside src. */
function target(from: string, spec: string): string | null {
  let abs: string;
  if (spec.startsWith('@/')) abs = join(SRC, spec.slice(2));
  else if (spec.startsWith('.')) abs = resolve(from, '..', spec);
  else return null;
  const rel = relative(SRC, abs);
  return rel.startsWith('..') ? null : (rel.split(/[\\/]/)[0] ?? null);
}

const forbidden: Record<string, string[]> = {
  app: ['admin'],
  admin: ['app'],
  shared: ['app', 'admin'],
};

describe('import rules', () => {
  for (const [area, banned] of Object.entries(forbidden)) {
    it(`src/${area} imports none of ${banned.join(', ')}`, () => {
      const bad: string[] = [];
      for (const file of files(join(SRC, area))) {
        const text = readFileSync(file, 'utf8');
        for (const m of text.matchAll(IMPORT)) {
          const spec = m[1] ?? m[2] ?? '';
          const top = target(file, spec);
          if (top !== null && banned.includes(top)) bad.push(`${relative(SRC, file)}: ${spec}`);
        }
      }
      expect(bad).toEqual([]);
    });
  }

  it('sees a cross import when there is one', () => {
    expect(target(join(SRC, 'app/pages/X.ts'), '@/admin/main.ts')).toBe('admin');
    expect(target(join(SRC, 'app/pages/X.ts'), '../../admin/main.ts')).toBe('admin');
    expect(target(join(SRC, 'app/pages/X.ts'), '@/shared/core/Dom.ts')).toBe('shared');
    expect(target(join(SRC, 'app/pages/X.ts'), 'vitest')).toBeNull();
  });
});
