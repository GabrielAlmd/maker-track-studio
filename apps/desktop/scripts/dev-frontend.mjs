import { spawnSync } from 'node:child_process';

const result = spawnSync('npm', ['run', 'dev', ...process.argv.slice(2)], {
  cwd: new URL('../../web', import.meta.url),
  env: { ...process.env, VITE_RUNTIME_MODE: 'desktop' },
  stdio: 'inherit',
  shell: process.platform === 'win32',
});

if (result.error) throw result.error;
process.exit(result.status ?? 1);
