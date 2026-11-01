import salesEn from './sales/translations/en';
import salesPtPT from './sales/translations/pt-PT';

const contributions = [{ en: salesEn, 'pt-PT': salesPtPT }];

function mergeTranslation(target, addition) {
  for (const [key, value] of Object.entries(addition)) {
    target[key] = value && typeof value === 'object' && !Array.isArray(value)
      ? mergeTranslation({ ...(target[key] ?? {}) }, value)
      : value;
  }
  return target;
}

export function translationsFor(locale, core, modules = contributions) {
  return modules.reduce((result, module) => mergeTranslation(result, module[locale] ?? {}), { ...core });
}
