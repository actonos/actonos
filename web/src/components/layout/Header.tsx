import { Menu, Search, Sun, Moon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { NavTab } from '@/components/layout/Sidebar';
import { useRealtime } from '@/components/providers/RealtimeProvider';
import { useTheme } from '@/components/providers/ThemeProvider';
import { NotificationBell } from '@/components/features/notifications/NotificationBell';

export interface HeaderProps {
  activeTab: NavTab;
  onOpenMobileSidebar: () => void;
  collapsed?: boolean;
  onLogout?: () => void;
  onOpenSearch: () => void;
  onNavigateTab?: (tab: NavTab) => void;
}

export function Header({ activeTab, onOpenMobileSidebar, onLogout, onOpenSearch, onNavigateTab }: HeaderProps) {
  const { t } = useTranslation(['nav', 'common']);
  const { snapshot } = useRealtime();
  const { resolvedTheme, toggleTheme } = useTheme();
  const metrics = snapshot?.metrics;

  const tabTitles: Record<NavTab, { title: string; category: string }> = {
    dashboard: { title: t('nav:links.dashboard'), category: t('nav:categories.overview') },
    agents: { title: t('nav:links.agents'), category: t('nav:categories.aiManagement') },
    'agent-studio': { title: t('nav:links.agentStudio'), category: t('nav:categories.configuration') },
    chat: { title: t('nav:links.chat'), category: t('nav:categories.conversation') },
    missions: { title: t('nav:links.missions'), category: t('nav:categories.operations') },
    operations: { title: t('nav:links.operations'), category: t('nav:categories.observability') },
    automations: { title: t('nav:links.automations'), category: t('nav:categories.scheduling') },
    plugins: { title: t('nav:links.plugins', 'Plugins'), category: t('nav:categories.extensions', 'Extensions') },
    tools: { title: t('nav:links.tools'), category: t('nav:categories.systemTools') },
    skills: { title: t('nav:links.skills'), category: t('nav:categories.agentSkills') },
    workspace: { title: t('nav:links.workspace'), category: t('nav:categories.files') },
    terminal: { title: t('nav:links.terminal', 'Terminal'), category: t('nav:categories.system') },
    notifications: { title: t('nav:links.notifications', 'Notifications'), category: t('nav:categories.system') },
    'audit-logs': { title: t('nav:links.audit-logs', 'Audit Logs'), category: t('nav:categories.system') },
    settings: { title: t('nav:links.settings'), category: t('nav:categories.system') },
  };

  const current = tabTitles[activeTab] || { title: 'ActonOS', category: 'Kernel' };

  return (
    <header className="h-16 px-4 sm:px-8 bg-canvas/80 backdrop-blur-md border-b border-onyx/10 sticky top-0 z-30 flex items-center justify-between">
      {/* Left: Mobile trigger & Page title */}
      <div className="flex items-center gap-3">
        <button
          onClick={onOpenMobileSidebar}
          className="lg:hidden p-2 rounded-full bg-soft-meadow border border-onyx/10 text-deep-ink hover:bg-canvas transition-colors cursor-pointer"
          aria-label={t('nav:sidebar.open')}
        >
          <Menu className="w-5 h-5" />
        </button>

        <img
          src={resolvedTheme === 'dark' ? '/actonos_logo_light.png' : '/actonos_logo.png'}
          alt="ActonOS"
          className="h-6 w-auto object-contain lg:hidden"
        />

        <div className="hidden lg:flex items-center gap-2">
          <span className="text-caption font-mono uppercase text-slate font-medium">
            {current.category}
          </span>
          <span className="text-slate font-mono">/</span>
          <h2 className="font-serif font-bold text-deep-ink text-body sm:text-heading-sm tracking-tight">
            {current.title}
          </h2>
        </div>
      </div>

      {/* Right: Telemetry mini badges & Lock Action */}
      <div className="flex items-center gap-2.5">
        <button
          type="button"
          onClick={onOpenSearch}
          className="hidden min-h-9 items-center gap-2 rounded-full border border-onyx/10 bg-soft-meadow px-3 text-caption text-slate transition-colors hover:bg-canvas hover:text-deep-ink sm:flex"
        >
          <Search className="h-3.5 w-3.5" />
          <span>{t('nav:search.trigger')}</span>
          <kbd className="rounded-full border border-onyx/10 bg-canvas px-1.5 py-0.5 font-mono text-[10px]">{t('nav:search.shortcut')}</kbd>
        </button>
        {metrics && (
          <div className="hidden md:flex items-center gap-3 px-3 py-2 bg-soft-meadow rounded-full border border-onyx/10 text-caption font-mono text-slate">
            <span>{t('nav:telemetry.cpu', { value: metrics.cpu?.usage_percent?.toFixed(0) || 0 })}</span>
            <span>•</span>
            <span>{t('nav:telemetry.ram', { value: metrics.memory?.used_mb || 0 })}</span>
          </div>
        )}

        <NotificationBell onNavigateTab={onNavigateTab} />

        {/* Theme Toggle Button */}
        <button
          type="button"
          onClick={toggleTheme}
          className="flex h-9 w-9 items-center justify-center rounded-full border border-onyx/10 bg-soft-meadow text-slate transition-colors hover:bg-canvas hover:text-deep-ink cursor-pointer"
          title={resolvedTheme === 'dark' ? t('nav:theme.switchToLight', 'Switch to light mode') : t('nav:theme.switchToDark', 'Switch to dark mode')}
          aria-label={t('nav:theme.toggle', 'Toggle theme')}
        >
          {resolvedTheme === 'dark' ? (
            <Sun className="h-4 w-4 text-hi-yellow" />
          ) : (
            <Moon className="h-4 w-4" />
          )}
        </button>

        {onLogout && (
          <button
            onClick={onLogout}
            className="flex items-center gap-1.5 px-3 py-2 bg-soft-meadow hover:bg-canvas text-slate hover:text-deep-ink rounded-full border border-onyx/10 text-caption font-sans font-medium transition-colors cursor-pointer"
            title={t('nav:session.lockTitle')}
          >
            <span>{t('nav:session.lock')}</span>
          </button>
        )}
      </div>
    </header>
  );
}
