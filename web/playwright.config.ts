import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
	testDir: 'e2e',
	fullyParallel: true,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 1 : 0,
	use: {
		baseURL: process.env.HASHI_E2E_BASE_URL ?? 'http://127.0.0.1:8080',
		trace: 'on-first-retry'
	},
	projects: [
		{
			name: 'chromium',
			use: { ...devices['Desktop Chrome'] }
		}
	],
	webServer: process.env.HASHI_E2E_BASE_URL
		? undefined
		: {
				command: 'pnpm run preview -- --host 127.0.0.1 --port 8080',
				port: 8080,
				reuseExistingServer: !process.env.CI
			}
});
