import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// base './': the interface is served under a random token path (internal/local), so every URL is relative.
export default defineConfig({
  plugins: [svelte()],
  base: './',
  build: { outDir: 'dist', emptyOutDir: true },
});
