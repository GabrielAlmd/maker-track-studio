const SEMVER_PATTERN = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$/;

export const semanticVersionMarker = '@@SEMANTIC_VERSION@@';

function firstVersionSource({ requested, tag, configured }) {
    for (const [source, value] of [['requested', requested], ['tag', tag], ['configured', configured]]) {
        if (value !== undefined && value !== null && value !== '') return { source, value };
    }
    throw new Error('No version was provided and no configured fallback version exists');
}

export function resolveBuildMetadata(sources) {
    const { source, value } = firstVersionSource(sources);
    if (typeof value !== 'string') throw new Error(`The ${source} version must be a string`);

    const semanticVersion = value.startsWith('v') ? value.slice(1) : value;
    const match = SEMVER_PATTERN.exec(semanticVersion);
    if (!match) {
        throw new Error(
            `Invalid ${source} version "${value}": expected SemVer X.Y.Z with optional prerelease/build metadata and at most one leading v`,
        );
    }

    const numericComponents = match.slice(1, 4);
    if (numericComponents.some(part => Number(part) > 65535)) {
        throw new Error(
            `Invalid ${source} version "${value}": Windows version components must be between 0 and 65535`,
        );
    }

    return {
        source,
        semanticVersion,
        windowsNumericVersion: numericComponents.join('.'),
    };
}

export function resolveVersion(sources) {
    return resolveBuildMetadata(sources).semanticVersion;
}

export function renderSemanticVersionTemplate(template, semanticVersion) {
    if (!template.includes(semanticVersionMarker)) {
        throw new Error(`Build template is missing ${semanticVersionMarker}`);
    }
    return template.replaceAll(semanticVersionMarker, semanticVersion);
}
