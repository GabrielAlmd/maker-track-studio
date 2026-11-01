import { cpSync, existsSync, mkdirSync, rmSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFileSync } from 'node:child_process';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const desktopDir = resolve(scriptDir, '..');
const webDir = resolve(desktopDir, '../web');
const webDistDir = resolve(webDir, 'dist');
const desktopDistDir = resolve(desktopDir, 'frontend', 'dist');

execFileSync('npm', ['run', 'build'], {
    cwd: webDir,
    stdio: 'inherit',
    shell: process.platform === 'win32',
    env: { ...process.env, VITE_RUNTIME_MODE: 'desktop' },
});

if (!existsSync(webDistDir)) {
    throw new Error(`Expected built frontend assets at ${webDistDir}`);
}

rmSync(desktopDistDir, { recursive: true, force: true });
mkdirSync(desktopDistDir, { recursive: true });
cpSync(webDistDir, desktopDistDir, { recursive: true });
