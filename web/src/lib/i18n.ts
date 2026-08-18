import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';

// English translations
import enCommon from '../locales/en/common.json';
import enNav from '../locales/en/nav.json';
import enAgents from '../locales/en/agents.json';
import enTools from '../locales/en/tools.json';
import enSkills from '../locales/en/skills.json';
import enSettings from '../locales/en/settings.json';
import enChat from '../locales/en/chat.json';
import enSetup from '../locales/en/setup.json';
import enWorkspace from '../locales/en/workspace.json';
import enIntegrations from '../locales/en/integrations.json';
import enChannels from '../locales/en/channels.json';
import enConnectors from '../locales/en/connectors.json';
import enDashboard from '../locales/en/dashboard.json';
import enMissions from '../locales/en/missions.json';
import enOperations from '../locales/en/operations.json';
import enAutomations from '../locales/en/automations.json';
import enAudit from '../locales/en/audit.json';

// Vietnamese translations
import viCommon from '../locales/vi/common.json';
import viNav from '../locales/vi/nav.json';
import viAgents from '../locales/vi/agents.json';
import viTools from '../locales/vi/tools.json';
import viSkills from '../locales/vi/skills.json';
import viSettings from '../locales/vi/settings.json';
import viChat from '../locales/vi/chat.json';
import viSetup from '../locales/vi/setup.json';
import viWorkspace from '../locales/vi/workspace.json';
import viIntegrations from '../locales/vi/integrations.json';
import viChannels from '../locales/vi/channels.json';
import viConnectors from '../locales/vi/connectors.json';
import viDashboard from '../locales/vi/dashboard.json';
import viMissions from '../locales/vi/missions.json';
import viOperations from '../locales/vi/operations.json';
import viAutomations from '../locales/vi/automations.json';
import viAudit from '../locales/vi/audit.json';

export const defaultNS = 'common';
export const resources = {
  en: {
    common: enCommon,
    nav: enNav,
    agents: enAgents,
    tools: enTools,
    skills: enSkills,
    settings: enSettings,
    chat: enChat,
    setup: enSetup,
    workspace: enWorkspace,
    integrations: enIntegrations,
    channels: enChannels,
    connectors: enConnectors,
    dashboard: enDashboard,
    missions: enMissions,
    operations: enOperations,
    automations: enAutomations,
    audit: enAudit,
  },
  vi: {
    common: viCommon,
    nav: viNav,
    agents: viAgents,
    tools: viTools,
    skills: viSkills,
    settings: viSettings,
    chat: viChat,
    setup: viSetup,
    workspace: viWorkspace,
    integrations: viIntegrations,
    channels: viChannels,
    connectors: viConnectors,
    dashboard: viDashboard,
    missions: viMissions,
    operations: viOperations,
    automations: viAutomations,
    audit: viAudit,
  },
} as const;

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    fallbackLng: 'en',
    defaultNS,
    ns: ['common', 'nav', 'agents', 'tools', 'skills', 'settings', 'chat', 'setup', 'workspace', 'integrations', 'channels', 'connectors', 'dashboard', 'missions', 'operations', 'automations', 'audit'],
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
