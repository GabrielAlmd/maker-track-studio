import { defineConfig, devices } from '@playwright/test';
import process from 'node:process';
import { existsSync } from 'node:fs';

const usePrebuiltWeb = process.env.MAKER_TRACK_PREBUILT_WEB === '1';
if (usePrebuiltWeb && !existsSync(new URL('./dist/index.html', import.meta.url))) {
  throw new Error('MAKER_TRACK_PREBUILT_WEB=1 requires the frontend artifact at dist/index.html');
}
const previewCommand = 'npm run preview -- --host 127.0.0.1 --port 4173';

export default defineConfig({
  testDir: './visual',
  testMatch: '**/*.screenshots.spec.js',
  fullyParallel: false,
  forbidOnly: true,
  retries: 0,
  workers: 1,
  reporter: 'list',
  snapshotPathTemplate: '{testDir}/../../../docs/screenshots/{arg}{ext}',
  expect: {
    toHaveScreenshot: {
      // Allow bounded font/caret rasterization drift. Material text/layout
      // changes exceed this ceiling and still fail the catalogue check.
      threshold: 0.1,
      maxDiffPixels: 600,
      animations: 'disabled',
      caret: 'hide',
    },
  },
  use: {
    ...devices['Desktop Chrome'],
    launchOptions: {
      args: ['--disable-gpu'],
    },
    baseURL: 'http://127.0.0.1:4173',
    viewport: { width: 1440, height: 960 },
    colorScheme: 'dark',
    locale: 'en-GB',
    timezoneId: 'Europe/Lisbon',
    reducedMotion: 'reduce',
  },
  webServer: {
    command: usePrebuiltWeb ? previewCommand : `npm run build && ${previewCommand}`,
    url: 'http://127.0.0.1:4173',
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});
