/* eslint-disable react-refresh/only-export-components */
import { createContext, useContext, useEffect, useState } from 'react';
import i18n from 'i18next';
import { I18nextProvider, initReactI18next, useTranslation } from 'react-i18next';
import en from './resources/en';
import ptPT from './resources/pt-PT';
import { translationsFor } from '../modules/translations';
import { getDesktopApp, isDesktopRuntime } from '../platform/runtime';

export const SUPPORTED_LOCALES = Object.freeze(['en', 'pt-PT']);
export const LOCALE_STORAGE_KEY = 'maker-track-studio-locale';

export function normalizeLocale(value) {
  const locale = String(value || '').trim().replace('_', '-').toLowerCase();
  if (locale === 'pt' || locale.startsWith('pt-pt')) return 'pt-PT';
  if (locale === 'en' || locale.startsWith('en-')) return 'en';
  return 'en';
}

if (!i18n.isInitialized) {
  i18n.use(initReactI18next).init({
    resources: { en: { translation: translationsFor('en', en) }, 'pt-PT': { translation: translationsFor('pt-PT', ptPT) } },
    lng: 'en', fallbackLng: 'en', supportedLngs: SUPPORTED_LOCALES,
    interpolation: { escapeValue: false }, returnNull: false,
  });
}

const LocaleContext = createContext(null);

function browserPreference() {
  const saved = window.localStorage.getItem(LOCALE_STORAGE_KEY);
  return { locale: normalizeLocale(saved || navigator.language), isSet: !!saved };
}

export function LocaleProvider({ children }) {
  const [state, setState] = useState({ ready: false, needsChoice: false, saving: false, error: '' });
  const desktopApp = getDesktopApp();
  const desktopGet = desktopApp?.GetLocalePreference;
  const desktopSet = desktopApp?.SetLocalePreference;

  useEffect(() => {
    let active = true;
    const load = async () => {
      try {
        const preference = typeof desktopGet === 'function' ? await desktopGet() : browserPreference();
        const locale = normalizeLocale(preference?.locale);
        await i18n.changeLanguage(locale);
        document.documentElement.lang = locale;
        if (active) setState({ ready: true, needsChoice: isDesktopRuntime() && typeof desktopGet === 'function' && !preference?.is_set, saving: false, error: '' });
      } catch {
        await i18n.changeLanguage('en');
        document.documentElement.lang = 'en';
        if (active) setState({ ready: true, needsChoice: isDesktopRuntime() && typeof desktopGet === 'function', saving: false, error: '' });
      }
    };
    load();
    return () => { active = false; };
  }, [desktopGet]);

  const setLocale = async (value) => {
    const locale = normalizeLocale(value);
    setState((current) => ({ ...current, saving: true, error: '' }));
    try {
      if (typeof desktopSet === 'function') await desktopSet(locale);
      window.localStorage.setItem(LOCALE_STORAGE_KEY, locale);
      await i18n.changeLanguage(locale);
      document.documentElement.lang = locale;
      setState({ ready: true, needsChoice: false, saving: false, error: '' });
      return true;
    } catch {
      setState((current) => ({ ...current, saving: false, error: 'locale.saveError' }));
      return false;
    }
  };

  const value = { ...state, locale: normalizeLocale(i18n.resolvedLanguage), setLocale };
  if (!state.ready) return <div role="status" className="p-8 ui-text-inverse">Loading…</div>;
  return <I18nextProvider i18n={i18n}><LocaleContext.Provider value={value}>{children}</LocaleContext.Provider></I18nextProvider>;
}

export function useLocale() {
  const context = useContext(LocaleContext);
  if (context) return context;
  return {
    locale: normalizeLocale(i18n.resolvedLanguage), saving: false, error: '', needsChoice: false,
    setLocale: async (value) => {
      const locale = normalizeLocale(value);
      window.localStorage.setItem(LOCALE_STORAGE_KEY, locale);
      await i18n.changeLanguage(locale);
      document.documentElement.lang = locale;
      return true;
    },
  };
}

export function FirstRunLanguageChoice({ children }) {
  const { t } = useTranslation();
  const locale = useLocale();
  const [selected, setSelected] = useState(locale.locale);
  if (!locale.needsChoice) return children;
  const submit = async (event) => { event.preventDefault(); await locale.setLocale(selected); };
  return <main className="flex min-h-screen items-center justify-center p-6 ui-surface-app">
    <section className="ui-modal w-full max-w-lg p-6" aria-labelledby="language-choice-title">
      <h1 id="language-choice-title" className="text-2xl font-bold ui-text-primary">{t('locale.title')}</h1>
      <p className="mt-2 ui-text-secondary">{t('locale.description')}</p>
      <form onSubmit={submit} className="mt-6 space-y-4">
        <fieldset className="grid gap-3" disabled={locale.saving}>
          {[['en', 'locale.english'], ['pt-PT', 'locale.portuguese']].map(([value, label]) => <label key={value} className="theme-option flex cursor-pointer items-center gap-3 p-4" data-selected={selected === value}>
            <input type="radio" name="locale" value={value} checked={selected === value} onChange={() => { setSelected(value); i18n.changeLanguage(value); }} />
            <span className="font-semibold ui-text-primary">{t(label)}</span>
          </label>)}
        </fieldset>
        {locale.error && <p role="alert" className="ui-text-danger">{t(locale.error)}</p>}
        <button type="submit" disabled={locale.saving} className="page-action w-full justify-center">{t(locale.saving ? 'locale.setting' : 'locale.continue')}</button>
      </form>
    </section>
  </main>;
}

export default i18n;
