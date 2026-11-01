// Static bundle of every vi namespace. Loaded on demand (see ../i18n.ts) so it ships as its own chunk.
import viCommon from '../../locales/vi/common.json';
import viNav from '../../locales/vi/nav.json';
import viAgents from '../../locales/vi/agents.json';
import viTools from '../../locales/vi/tools.json';
import viSkills from '../../locales/vi/skills.json';
import viSettings from '../../locales/vi/settings.json';
import viChat from '../../locales/vi/chat.json';
import viSetup from '../../locales/vi/setup.json';
import viWorkspace from '../../locales/vi/workspace.json';
import viDashboard from '../../locales/vi/dashboard.json';
import viMissions from '../../locales/vi/missions.json';
import viOperations from '../../locales/vi/operations.json';
import viAutomations from '../../locales/vi/automations.json';
import viAudit from '../../locales/vi/audit.json';
import viNotifications from '../../locales/vi/notifications.json';
import viTerminal from '../../locales/vi/terminal.json';
import viPlugins from '../../locales/vi/plugins.json';
import viChannels from '../../locales/vi/channels.json';
import viCosts from '../../locales/vi/costs.json';
import viMailcal from '../../locales/vi/mailcal.json';

export const bundle = {
  common: viCommon,
  nav: viNav,
  agents: viAgents,
  tools: viTools,
  skills: viSkills,
  settings: viSettings,
  chat: viChat,
  setup: viSetup,
  workspace: viWorkspace,
  dashboard: viDashboard,
  missions: viMissions,
  operations: viOperations,
  automations: viAutomations,
  audit: viAudit,
  notifications: viNotifications,
  terminal: viTerminal,
  plugins: viPlugins,
  channels: viChannels,
  costs: viCosts,
  mailcal: viMailcal,
} as const;

export default bundle;
