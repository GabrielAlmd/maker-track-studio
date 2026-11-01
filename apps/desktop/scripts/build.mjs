import { appendFileSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFileSync } from 'node:child_process';
import { renderSemanticVersionTemplate, resolveBuildMetadata } from './build-metadata.mjs';

const desktopDir = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const configPath = resolve(desktopDir, 'wails.json');
const windowsInfoPath = resolve(desktopDir, 'build/windows/info.json');
const nsisProjectPath = resolve(desktopDir, 'build/windows/installer/project.nsi');
const originalConfig = readFileSync(configPath, 'utf8');
const originalWindowsInfo = readFileSync(windowsInfoPath, 'utf8');
const originalNsisProject = readFileSync(nsisProjectPath, 'utf8');
const config = JSON.parse(originalConfig);
const args = process.argv.slice(2);
let requested = process.env.BUILD_VERSION;
if (args[0] === '--version') {
    requested = args[1];
    if (!requested) throw new Error('--version requires a semantic version');
    args.splice(0, 2);
}
const gitTag = process.env.GITHUB_REF_TYPE === 'tag' ? process.env.GITHUB_REF_NAME : undefined;
const { semanticVersion, windowsNumericVersion, source } = resolveBuildMetadata({
    requested,
    tag: gitTag,
    configured: config.info.productVersion,
});
const commit = execFileSync('git', ['rev-parse', 'HEAD'], { cwd: desktopDir, encoding: 'utf8' }).trim();
if (!/^[a-f0-9]{40,64}$/.test(commit)) throw new Error('Invalid Git commit');
if (args.some(arg => /^--?(ldflags|o|output|project)(=|$)/.test(arg))) {
    throw new Error('Build identity flags are managed by this script');
}
config.info.productVersion = windowsNumericVersion;
config.info.comments = `Application version: ${semanticVersion}; Commit: ${commit}`;

console.log([
    `Git ref:          ${process.env.GITHUB_REF || '(local checkout)'}`,
    `Git tag:          ${gitTag || '(none)'}`,
    `Version source:   ${source}`,
    `App version:      ${semanticVersion}`,
    `Windows version:  ${windowsNumericVersion}`,
    `Commit:           ${commit}`,
].join('\n'));

try {
    writeFileSync(configPath, JSON.stringify(config, null, 4) + '\n');
    writeFileSync(windowsInfoPath, renderSemanticVersionTemplate(originalWindowsInfo, semanticVersion));
    writeFileSync(nsisProjectPath, renderSemanticVersionTemplate(originalNsisProject, semanticVersion));
    execFileSync('go', ['run', 'github.com/wailsapp/wails/v2/cmd/wails@v2.12.0',
        'build',
        ...args, '-ldflags', `-X main.version=${semanticVersion} -X main.commit=${commit}`,
    ], { cwd: desktopDir, stdio: 'inherit' });
} finally {
    writeFileSync(configPath, originalConfig);
    writeFileSync(windowsInfoPath, originalWindowsInfo);
    writeFileSync(nsisProjectPath, originalNsisProject);
}
if (process.env.GITHUB_ENV) {
    appendFileSync(process.env.GITHUB_ENV,
        `BUILD_VERSION=${semanticVersion}\nWINDOWS_NUMERIC_VERSION=${windowsNumericVersion}\nBUILD_COMMIT=${commit}\nRELEASE_ID=${semanticVersion}\n`);
}
console.log(`Built Maker Track Studio ${semanticVersion} (${commit})`);
