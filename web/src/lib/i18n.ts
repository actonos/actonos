import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';

// English translations
import enCommon from '../locales/en/common.json';
import enNav from '../locales/en/nav.json';
import enAgents from '../locales/en/agents.json';
import enTools from '../locales/en/tools.json';
import enSettings from '../locales/en/settings.json';
import enChat from '../locales/en/chat.json';
import enSetup from '../locales/en/setup.json';
import enWorkspace from '../locales/en/workspace.json';
import enIntegrations from '../locales/en/integrations.json';

// Vietnamese translations
import viCommon from '../locales/vi/common.json';
import viNav from '../locales/vi/nav.json';
import viAgents from '../locales/vi/agents.json';
import viTools from '../locales/vi/tools.json';
import viSettings from '../locales/vi/settings.json';
import viChat from '../locales/vi/chat.json';
import viSetup from '../locales/vi/setup.json';
import viWorkspace from '../locales/vi/workspace.json';
import viIntegrations from '../locales/vi/integrations.json';

export const defaultNS = 'common';
export const resources = {
  en: {
    common: enCommon,
    nav: enNav,
    agents: enAgents,
    tools: enTools,
    settings: enSettings,
    chat: enChat,
    setup: enSetup,
    workspace: enWorkspace,
    integrations: enIntegrations,
  },
  vi: {
    common: viCommon,
    nav: viNav,
    agents: viAgents,
    tools: viTools,
    settings: viSettings,
    chat: viChat,
    setup: viSetup,
    workspace: viWorkspace,
    integrations: viIntegrations,
  },
} as const;

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    fallbackLng: 'en',
    defaultNS,
    ns: ['common', 'nav', 'agents', 'tools', 'settings', 'chat', 'setup', 'workspace', 'integrations'],
    resources,
    interpolation: {
      escapeValue: false,
    },
    detection: {
      order: ['localStorage', 'navigator'],
      caches: ['localStorage'],
    },
  });

export default i18n;
