import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vitest/config';

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			preprocess: vitePreprocess(),
			compilerOptions: {
				// Runes everywhere except dependencies.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},

			// SPA: every unknown path serves index.html; the Go server does the same (internal/web).
			adapter: adapter({
				pages: '../internal/web/dist',
				assets: '../internal/web/dist',
				fallback: 'index.html',
				strict: true
			})
		})
	],
	server: {
		// `make dev`: API calls go to the Go backend.
		proxy: {
			'/api': 'http://127.0.0.1:8080',
			'/healthz': 'http://127.0.0.1:8080',
			'/readyz': 'http://127.0.0.1:8080'
		}
	},
	test: {
		expect: { requireAssertions: true },
		include: ['src/**/*.{test,spec}.{js,ts}']
	}
});
