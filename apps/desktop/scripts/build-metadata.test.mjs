import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
    renderSemanticVersionTemplate,
    resolveBuildMetadata,
    resolveVersion,
} from './build-metadata.mjs';

test('Windows version resource includes the string FileVersion required by .NET', () => {
    const template = readFileSync(new URL('../build/windows/info.json', import.meta.url), 'utf8');
    const scenarios = [
        { semanticVersion: '1.0.0', windowsNumericVersion: '1.0.0' },
        { semanticVersion: '2.3.4-rc.1', windowsNumericVersion: '2.3.4' },
        { semanticVersion: '65535.0.0+build.5', windowsNumericVersion: '65535.0.0' },
    ];
    for (const { semanticVersion, windowsNumericVersion } of scenarios) {
        const info = {
            ProductVersion: windowsNumericVersion,
            ProductName: 'Maker Track Studio',
            CompanyName: 'Maker Track Studio',
            Copyright: '',
            Comments: 'Commit: abc123',
        };
        const rendered = renderSemanticVersionTemplate(template, semanticVersion);
        const resource = JSON.parse(rendered.replace(/\{\{\.Info\.(\w+)\}\}/g, (_, key) => {
            assert.ok(Object.hasOwn(info, key), `Unknown template field: ${key}`);
            return info[key];
        }));
        assert.equal(resource.fixed.file_version, windowsNumericVersion);
        assert.equal(resource.fixed.product_version, windowsNumericVersion);
        assert.ok(Object.keys(resource.info).length > 0);
        for (const table of Object.values(resource.info)) {
            // Without this string, .NET tries other language tables and can
            // overwrite otherwise valid ProductName/ProductVersion with blanks.
            assert.equal(table.FileVersion, windowsNumericVersion);
            assert.equal(table.ProductVersion, semanticVersion);
            assert.equal(table.ProductName, info.ProductName);
        }
    }
});

test('manual version, release tag, then configured version', () => {
    assert.equal(resolveVersion({ requested: 'v2.3.4-rc.1', tag: 'v1.2.3', configured: '1.0.0' }), '2.3.4-rc.1');
    assert.equal(resolveVersion({ tag: 'v1.2.3-beta.1', configured: '1.0.0' }), '1.2.3-beta.1');
    assert.equal(resolveVersion({ configured: '1.0.0' }), '1.0.0');
});

test('normalizes release and prerelease versions with an optional leading v', () => {
    const versions = [
        ['0.2.0', '0.2.0'],
        ['v0.2.0', '0.2.0'],
        ['0.1.0-beta.1', '0.1.0-beta.1'],
        ['v0.1.0-beta.1', '0.1.0-beta.1'],
        ['1.0.0-alpha', '1.0.0-alpha'],
        ['1.0.0-rc.1', '1.0.0-rc.1'],
        ['v1.2.3-rc.2', '1.2.3-rc.2'],
        ['1.2.3+build.5', '1.2.3+build.5'],
    ];
    for (const [input, expected] of versions) {
        assert.equal(resolveVersion({ configured: input }), expected);
    }
});

test('derives a numeric Windows version without losing the application SemVer', () => {
    const metadata = resolveBuildMetadata({ tag: 'v0.1.0-beta.1' });
    assert.equal(metadata.semanticVersion, '0.1.0-beta.1');
    assert.equal(metadata.windowsNumericVersion, '0.1.0');
});

test('rejects malformed, unsafe, and Windows-incompatible versions with a useful error', () => {
    const invalid = [
        'vv0.2.0', 'v', '1', '1.2', '1.2.3.4', '01.2.3', '1.02.3', '1.2.03', '1.2.x', '1.2.3-01', '65536.0.0', '1.2.3 -X main.commit=fake', '1.2.3\nBAD=value',
    ];
    for (const configured of invalid) {
        assert.throws(
            () => resolveVersion({ configured }),
            /Invalid configured version/,
        );
    }
    assert.equal(resolveVersion({ configured: '65535.0.0' }), '65535.0.0');
    assert.throws(() => resolveVersion({}), /No version was provided/);
});

test('NSIS keeps numeric fixed fields and exposes the semantic display version', () => {
    const template = readFileSync(new URL('../build/windows/installer/project.nsi', import.meta.url), 'utf8');
    const rendered = renderSemanticVersionTemplate(template, '0.1.0-beta.1');
    assert.match(rendered, /VIProductVersion "\$\{INFO_PRODUCTVERSION\}\.0"/);
    assert.match(rendered, /VIAddVersionKey "ProductVersion"  "\$\{INFO_SEMANTICVERSION\}"/);
    assert.match(rendered, /!define INFO_SEMANTICVERSION "0\.1\.0-beta\.1"/);
});

test('release artifact names use the normalized semantic version', () => {
    const buildScript = readFileSync(new URL('./build.mjs', import.meta.url), 'utf8');
    assert.match(buildScript, /RELEASE_ID=\$\{semanticVersion\}/);

    const workflow = readFileSync(new URL('../../../.github/workflows/desktop-packaging.yml', import.meta.url), 'utf8');
    for (const artifact of [
        'maker-track-studio-linux-amd64-${RELEASE_ID}.tar.gz',
        'maker-track-studio-windows-amd64-$env:RELEASE_ID.zip',
        'maker-track-studio-windows-amd64-$env:RELEASE_ID-installer.exe',
    ]) {
        assert.ok(workflow.includes(artifact), `Missing semantic release artifact: ${artifact}`);
    }
});

test('Wails fallback metadata uses the first public release version', () => {
    const config = JSON.parse(readFileSync(new URL('../wails.json', import.meta.url), 'utf8'));
    assert.equal(config.info.productVersion, '1.0.0');
});

test('font license notice is included in both desktop package formats', () => {
    const workflow = readFileSync(new URL('../../../.github/workflows/desktop-packaging.yml', import.meta.url), 'utf8');
    const installer = readFileSync(new URL('../build/windows/installer/project.nsi', import.meta.url), 'utf8');
    assert.equal((workflow.match(/THIRD_PARTY_FONTS_OFL-1\.1\.txt/g) || []).length, 2);
    assert.match(installer, /MUI_PAGE_LICENSE .*THIRD_PARTY_FONTS_OFL-1\.1\.txt/);
});
