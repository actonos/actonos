import i18n, { type BackendModule } from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';

export const defaultNS = 'common';

export const namespaces = [
  'common',
  'nav',
  'agents',
  'tools',
  'skills',
  'settings',
  'chat',
  'setup',
  'workspace',
  'dashboard',
  'missions',
  'operations',
  'automations',
  'audit',
  'notifications',
  'terminal',
  'plugins',
  'channels',
  'costs',
] as const;

type LocaleBundle = Record<string, Record<string, unknown>>;

// Each language is its own chunk, fetched only when it is the active or fallback
// language, instead of shipping every locale inside the entry bundle.
const bundleLoaders: Record<string, () => Promise<{ default: LocaleBundle }>> = {
  en: () => import('./locale-bundles/en'),
  vi: () => import('./locale-bundles/vi'),
};

const lazyLocaleBackend: BackendModule = {
  type: 'backend',
  init() {},
  read(language, namespace, callback) {
    const load = bundleLoaders[language];
    if (!load) {
      // Region variants such as "en-US" resolve through their base language.
      callback(null, {});
      return;
    }
    load().then(
      (module) => callback(null, module.default[namespace] ?? {}),
      (error: Error) => callback(error, false),
    );
  },
};

/**
 * Resolves once the detected and fallback languages are loaded. Await it before
 * the first render (and in tests) so components never see raw translation keys.
 */
export const i18nReady: Promise<unknown> = i18n
  .use(LanguageDetector)
  .use(lazyLocaleBackend)
  .use(initReactI18next)
  .init({
    fallbackLng: 'en',
    defaultNS,
    ns: [...namespaces],
    interpolation: {
      escapeValue: false,
    },
    detection: {
      order: ['localStorage', 'navigator'],
      caches: ['localStorage'],
    },
  });

export default i18n;
