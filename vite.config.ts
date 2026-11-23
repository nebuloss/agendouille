import { defineConfig } from 'vite';
import { resolve } from 'node:path';

// Two bundles, built one after the other (`npm run build`): the family web
// app and the admin UI. Each is self-contained, with its own copy of
// src/shared, so the admin code never sits in a chunk the public listener
// serves (PLAN.md 9.1). Each lands where Go's embed picks it up; the file
// names are fixed because the layouts name them, and the server
// revalidates them by ETag.
const BUNDLES = ['app', 'admin'] as const;
type Bundle = (typeof BUNDLES)[number];

export default defineConfig(({ mode }) => {
  if (!(BUNDLES as readonly string[]).includes(mode)) {
    throw new Error(`build with --mode app or --mode admin, not ${JSON.stringify(mode)}`);
  }
  const bundle = mode as Bundle;
  return {
    resolve: { alias: { '@': resolve(import.meta.dirname, './src') } },
    build: {
      outDir: resolve(import.meta.dirname, `internal/web/assets/static/${bundle}`),
      // Not emptied: the directory holds the committed README.txt that
      // embed needs on a clean checkout.
      emptyOutDir: false,
      target: 'es2022',
      cssCodeSplit: false,
      lib: {
        entry: resolve(import.meta.dirname, `src/${bundle}/main.ts`),
        formats: ['es'],
        fileName: () => `${bundle}.js`,
      },
    },
  };
});
