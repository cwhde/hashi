import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	compilerOptions: {
		// Runes everywhere except dependencies.
		runes: ({ filename }) => (filename.split(/[/\\]/).includes('node_modules') ? undefined : true)
	},
	kit: {
		// SPA: every unknown path serves index.html; the Go server does the same (internal/web).
		adapter: adapter({
			pages: '../internal/web/dist',
			assets: '../internal/web/dist',
			fallback: 'index.html',
			strict: true
		})
	}
};

export default config;
