import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vitest/config';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
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
