import { existsSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { defineConfig } from '@playwright/test';

const configDirectory = dirname(fileURLToPath(import.meta.url));
const port = Number(process.env.PLAYWRIGHT_PORT ?? 4187);
const basePath = '/personal-assistant/';
const baseURL = `http://127.0.0.1:${port}`;
const browserExecutable =
  process.env.CHROMIUM_PATH ??
  ['/repl/tools/bin/chromium', '/usr/bin/chromium', '/usr/bin/chromium-browser'].find(
    (candidate) => existsSync(candidate),
  );

export default defineConfig({
  testDir: './tests/browser',
  fullyParallel: false,
  workers: 1,
  reporter: 'list',
  timeout: 30_000,
  expect: { timeout: 8_000 },
  outputDir: './test-results/profile-google-accounts',
  use: {
    baseURL,
    browserName: 'chromium',
    headless: true,
    launchOptions: {
      ...(browserExecutable ? { executablePath: browserExecutable } : {}),
      args: ['--no-sandbox'],
    },
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
  },
  webServer: {
    command: `PORT=${port} BASE_PATH=${basePath} pnpm exec vite --config vite.config.ts --host 127.0.0.1`,
    cwd: resolve(configDirectory),
    url: `${baseURL}${basePath}`,
    reuseExistingServer: false,
    timeout: 60_000,
  },
});
