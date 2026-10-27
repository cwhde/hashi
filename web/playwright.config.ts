import { defineConfig } from '@playwright/test';

// E2E against a real Hashi binary built with `-tags dev` (make test-e2e). globalSetup starts it on a free port with a
// fresh data dir and master key, and shares the base URL and bootstrap credentials through e2e/.state.json.
export default defineConfig({
	testDir: 'e2e',
	globalSetup: './e2e/global-setup.ts',
	fullyParallel: false,
	workers: 1,
	retries: 0,
	reporter: process.env.CI ? 'line' : 'list',
	use: { trace: 'retain-on-failure' },
	projects: [{ name: 'chromium', use: { browserName: 'chromium' } }]
});
