// Static bundle of every en namespace. Loaded on demand (see ../i18n.ts) so it ships as its own chunk.
import enCommon from '../../locales/en/common.json';
import enNav from '../../locales/en/nav.json';
import enAgents from '../../locales/en/agents.json';
import enTools from '../../locales/en/tools.json';
import enSkills from '../../locales/en/skills.json';
import enSettings from '../../locales/en/settings.json';
import enChat from '../../locales/en/chat.json';
import enSetup from '../../locales/en/setup.json';
import enWorkspace from '../../locales/en/workspace.json';
import enDashboard from '../../locales/en/dashboard.json';
import enMissions from '../../locales/en/missions.json';
import enOperations from '../../locales/en/operations.json';
import enAutomations from '../../locales/en/automations.json';
import enAudit from '../../locales/en/audit.json';
import enNotifications from '../../locales/en/notifications.json';
import enTerminal from '../../locales/en/terminal.json';
import enPlugins from '../../locales/en/plugins.json';
import enChannels from '../../locales/en/channels.json';
import enCosts from '../../locales/en/costs.json';

export const bundle = {
  common: enCommon,
  nav: enNav,
  agents: enAgents,
  tools: enTools,
  skills: enSkills,
  settings: enSettings,
  chat: enChat,
  setup: enSetup,
  workspace: enWorkspace,
  dashboard: enDashboard,
  missions: enMissions,
  operations: enOperations,
  automations: enAutomations,
  audit: enAudit,
  notifications: enNotifications,
  terminal: enTerminal,
  plugins: enPlugins,
  channels: enChannels,
  costs: enCosts,
} as const;

export default bundle;
