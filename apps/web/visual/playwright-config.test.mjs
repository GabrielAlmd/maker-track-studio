import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { test } from 'node:test';

function loadConfig(t, { prebuilt, artifact = false }) {
  const root = mkdtempSync(join(tmpdir(), 'maker-playwright-config-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const config = join(root, 'config.mjs');
  copyFileSync(new URL('../playwright.screenshots.config.js', import.meta.url), config);
  symlinkSync(fileURLToPath(new URL('../node_modules', import.meta.url)), join(root, 'node_modules'), 'dir');
  if (artifact) {
    mkdirSync(join(root, 'dist'));
    writeFileSync(join(root, 'dist/index.html'), '<!doctype html><title>Disposable fixture</title>');
  }
  const env = { ...process.env, CI: 'true' };
  delete env.MAKER_TRACK_PREBUILT_WEB;
  delete env.NODE_TEST_CONTEXT;
  if (prebuilt !== undefined) env.MAKER_TRACK_PREBUILT_WEB = prebuilt;
  const result = spawnSync(process.execPath, ['--input-type=module', '-e',
    'const {default: config} = await import(process.argv[1]); const {writeFileSync} = await import("node:fs"); writeFileSync(process.argv[2], config.webServer.command);',
    pathToFileURL(config).href, join(root, 'command.txt')], { env, encoding: 'utf8' });
  const command = existsSync(join(root, 'command.txt')) ? readFileSync(join(root, 'command.txt'), 'utf8') : '';
  return { result, root, command };
}

test('normal screenshot runs build before preview', (t) => {
  const { result, command } = loadConfig(t, {});
  assert.equal(result.status, 0, result.stderr);
  assert.equal(command, 'npm run build && npm run preview -- --host 127.0.0.1 --port 4173');
});

test('prebuilt mode serves the received artifact without rebuilding', (t) => {
  const { result, command } = loadConfig(t, { prebuilt: '1', artifact: true });
  assert.equal(result.status, 0, result.stderr);
  assert.equal(command, 'npm run preview -- --host 127.0.0.1 --port 4173');
});

test('a missing artifact fails prebuilt mode without a fallback build', (t) => {
  const { result, root } = loadConfig(t, { prebuilt: '1' });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /requires the frontend artifact at dist\/index.html/);
  assert.equal(existsSync(join(root, 'dist')), false);
});
