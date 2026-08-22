import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { LanguageSwitcher } from '@/components/ui/LanguageSwitcher';
import { SUPPORTED_LANGUAGES } from '@/components/ui/LanguageSelectModal';
import { useRealtime } from '@/components/providers/RealtimeProvider';
import { ConnectionStatusIndicator } from '@/components/features/telemetry/ConnectionStatusIndicator';
import {
  LayoutDashboard,
  Bot,
  MessageSquare,
  Calendar,
  Wrench,
  Sparkles,
  Folder,
  Sliders,
  ChevronLeft,
  ChevronRight,
  X,
  Radio,
  Link2,
  Globe,
  Search,
  Check,
  Target,
  Gauge,
  ShieldCheck,
  Bell,
  Terminal,
} from 'lucide-react';
import { useTheme } from '../providers/ThemeProvider';

export type NavTab =
  | 'dashboard'
  | 'agents'
  | 'agent-studio'
  | 'chat'
  | 'missions'
  | 'operations'
  | 'automations'
  | 'tools'
  | 'skills'
  | 'workspace'
  | 'terminal'
  | 'channels'
  | 'connectors'
  | 'notifications'
  | 'audit-logs'
  | 'settings';

interface NavItem {
  id: NavTab;
  label: string;
  icon: React.ElementType;
}

interface NavSection {
  label?: string;
  items: NavItem[];
}

export interface SidebarProps {
  activeTab: NavTab;
  onSelectTab: (tab: NavTab) => void;
  collapsed: boolean;
  onToggleCollapse: () => void;
  mobileOpen: boolean;
  onCloseMobile: () => void;
}

export function Sidebar({
  activeTab,
  onSelectTab,
  collapsed,
  onToggleCollapse,
  mobileOpen,
  onCloseMobile,
}: SidebarProps) {
  const { t, i18n } = useTranslation('nav');
  const { snapshot } = useRealtime();
  const unreadCount = snapshot?.notifications_unread ?? 0;
  const [showLangOverlay, setShowLangOverlay] = useState(false);
  const [langSearch, setLangSearch] = useState('');
  const { resolvedTheme } = useTheme();

  const currentLangCode = (i18n.language || 'en').split('-')[0].toLowerCase();

  const sections: NavSection[] = [
    {
      label: t('sections.overview'),
      items: [
        { id: 'dashboard', label: t('links.dashboard', 'Dashboard'), icon: LayoutDashboard },
        { id: 'missions', label: t('links.missions', 'Missions'), icon: Target },
        { id: 'operations', label: t('links.operations', 'Live Operations'), icon: Gauge },
      ],
    },
    {
      label: t('sections.build'),
      items: [
        { id: 'agents', label: t('links.agents', 'Agents'), icon: Bot },
        { id: 'chat', label: t('links.chat', 'Chat'), icon: MessageSquare },
        { id: 'automations', label: t('links.automations', 'Automations'), icon: Calendar },
        { id: 'workspace', label: t('links.workspace', 'Workspace'), icon: Folder },
        { id: 'terminal', label: t('links.terminal', 'Terminal'), icon: Terminal },
      ],
    },
    {
      label: t('sections.connections', 'Connections'),
      items: [
        { id: 'channels', label: t('links.channels', 'Chat Channels'), icon: Radio },
        { id: 'connectors', label: t('links.connectors', 'Connectors'), icon: Link2 },
      ],
    },
    {
      label: t('sections.capabilities'),
      items: [
        { id: 'tools', label: t('links.tools', 'Tools'), icon: Wrench },
        { id: 'skills', label: t('links.skills', 'Skills'), icon: Sparkles },
      ],
    },
    {
      label: t('sections.system', 'System'),
      items: [
        { id: 'notifications', label: t('links.notifications', 'Notifications'), icon: Bell },
        { id: 'audit-logs', label: t('links.audit-logs', 'Audit Logs'), icon: ShieldCheck },
        { id: 'settings', label: t('links.settings', 'Settings'), icon: Sliders },
      ],
    },
  ];

  const handleSelect = (tab: NavTab) => {
    onSelectTab(tab);
    onCloseMobile();
  };

  const filteredLanguages = SUPPORTED_LANGUAGES.filter(
    (l) =>
      l.name.toLowerCase().includes(langSearch.toLowerCase()) ||
      l.nativeName.toLowerCase().includes(langSearch.toLowerCase()) ||
      l.code.toLowerCase().includes(langSearch.toLowerCase())
  );

  return (
    <>
      {/* Mobile Drawer Overlay */}
      {mobileOpen && (
        <div
          className="fixed inset-0 bg-black/40 backdrop-blur-xs z-40 lg:hidden transition-opacity"
          onClick={onCloseMobile}
        />
      )}

      {/* Sidebar Container */}
      <aside
        className={`fixed top-0 left-0 bottom-0 z-50 bg-soft-meadow border-r border-onyx/10 flex flex-col justify-between transition-all duration-200 ease-in-out ${collapsed && !showLangOverlay ? 'w-20' : 'w-64'
          } ${mobileOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0'}`}
      >
        {/* Top: Logo & Title */}
        <div>
          <div className="h-16 px-4 flex items-center justify-between border-b border-onyx/10">
            <div
              className={`flex items-center gap-3 cursor-pointer select-none overflow-hidden ${collapsed && !showLangOverlay ? 'justify-center w-full' : ''
                }`}
              onClick={() => handleSelect('dashboard')}
            >
              {collapsed && !showLangOverlay ? (
                <img
                  src="/actonos_icon.png"
                  alt="ActonOS"
                  className="h-8 w-8 object-contain shrink-0"
                />
              ) : (
                <img
                  src={resolvedTheme === 'dark' ? '/actonos_logo_light.png' : '/actonos_logo.png'}
                  alt="ActonOS"
                  className="h-8 w-auto max-w-[190px] object-contain shrink-0"
                />
              )}
            </div>

            {/* Mobile close button */}
            <button
              onClick={onCloseMobile}
              aria-label={t('sidebar.close')}
              title={t('sidebar.close')}
              className="lg:hidden p-1.5 rounded-full hover:bg-black/5 text-deep-ink"
            >
              <X className="w-5 h-5" />
            </button>
          </div>

          {/* Navigation Links with Sections */}
          <nav className="p-3 space-y-1 overflow-y-auto max-h-[calc(100vh-200px)]">
            {sections.map((section, sIdx) => (
              <div key={sIdx}>
                {/* Section Divider Label */}
                {section.label && (
                  <div className={`flex items-center gap-2 mt-4 mb-2 ${collapsed && !showLangOverlay ? 'justify-center' : 'px-3'}`}>
                    {!collapsed || showLangOverlay ? (
                      <span className="text-caption font-semibold uppercase tracking-wider text-slate select-none">
                        {section.label}
                      </span>
                    ) : (
                      <div className="w-8 border-t border-onyx/10" />
                    )}
                  </div>
                )}

                {/* Section Items */}
                <div className="space-y-0.5">
                  {section.items.map((item) => {
                    const Icon = item.icon;
                    const isActive = activeTab === item.id;
                    return (
                      <button
                        key={item.id}
                        onClick={() => handleSelect(item.id)}
                        className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-2xl transition-all duration-150 group cursor-pointer ${isActive
                          ? 'bg-deep-ink text-white font-semibold shadow-xs'
                          : 'text-slate hover:text-deep-ink hover:bg-black/5 dark:hover:bg-white/5'
                          } ${collapsed && !showLangOverlay ? 'justify-center' : ''}`}
                        title={collapsed && !showLangOverlay ? item.label : undefined}
                      >
                        <div
                          className={`w-8 h-8 rounded-full flex items-center justify-center shrink-0 transition-colors relative ${isActive
                            ? 'bg-white/15 text-hi-yellow'
                            : 'bg-canvas text-slate group-hover:text-deep-ink group-hover:bg-canvas/90 shadow-2xs'
                            }`}
                        >
                          <Icon className="w-4 h-4" />
                          {collapsed && !showLangOverlay && item.id === 'notifications' && unreadCount > 0 && (
                            <span className="absolute -top-0.5 -right-0.5 w-2.5 h-2.5 rounded-full bg-red-500 ring-2 ring-soft-meadow" />
                          )}
                        </div>

                        {(!collapsed || showLangOverlay) && (
                          <span className="text-body-sm font-medium leading-snug truncate flex-1 text-left">
                            {item.label}
                          </span>
                        )}

                        {(!collapsed || showLangOverlay) && item.id === 'notifications' && unreadCount > 0 && (
                          <span className="px-1.5 py-0.5 rounded-full bg-red-500 text-white text-[10px] font-mono font-bold">
                            {unreadCount > 99 ? '99+' : unreadCount}
                          </span>
                        )}
                      </button>
                    );
                  })}
                </div>
              </div>
            ))}
          </nav>
        </div>

        {/* Bottom: System Status, Language & Collapse button */}
        <div className="p-3 border-t border-onyx/10 space-y-2 bg-soft-meadow">
          {/* Live Status Pill & Telemetry Popover */}
          <ConnectionStatusIndicator
            compact={collapsed && !showLangOverlay}
            placement="sidebar"
          />

          {/* Language Switcher & Collapse Toggle */}
          <div className={`flex items-center ${collapsed && !showLangOverlay ? 'flex-col gap-2 justify-center' : 'justify-between gap-2'}`}>
            <LanguageSwitcher
              onClick={() => setShowLangOverlay(true)}
              compact={collapsed && !showLangOverlay}
            />

            <button
              onClick={onToggleCollapse}
              className="hidden lg:flex items-center justify-center w-9 h-9 p-0 rounded-full hover:bg-canvas text-slate hover:text-deep-ink transition-colors border border-onyx/10 cursor-pointer shrink-0"
              title={collapsed ? t('sidebar.expand') : t('sidebar.collapse')}
            >
              {collapsed && !showLangOverlay ? <ChevronRight className="w-4 h-4" /> : <ChevronLeft className="w-4 h-4" />}
            </button>
          </div>
        </div>

        {/* Sidebar-scoped Language Selection Overlay */}
        {showLangOverlay && (
          <div className="absolute inset-0 z-50 bg-soft-meadow flex flex-col justify-between p-4 animate-in fade-in duration-150">
            <div className="flex-1 flex flex-col min-h-0">
              {/* Header */}
              <div className="flex items-center justify-between pb-3 mb-3 border-b border-onyx/10 shrink-0">
                <div className="flex items-center gap-2">
                  <Globe className="w-4 h-4 text-deep-ink" />
                  <h3 className="font-serif font-bold text-body text-deep-ink">{t('language.title')}</h3>
                </div>
                <button
                  onClick={() => setShowLangOverlay(false)}
                  className="p-1.5 rounded-full hover:bg-black/5 text-deep-ink transition-colors cursor-pointer"
                  title={t('language.close')}
                >
                  <X className="w-4 h-4" />
                </button>
              </div>

              {/* Search Input */}
              <div className="relative mb-3 shrink-0">
                <Search className="w-3.5 h-3.5 text-slate absolute left-3 top-1/2 -translate-y-1/2" />
                <input
                  type="text"
                  placeholder={t('language.search')}
                  value={langSearch}
                  onChange={(e) => setLangSearch(e.target.value)}
                  className="w-full bg-canvas text-deep-ink pl-8 pr-3 py-1.5 rounded-full border border-onyx/10 text-caption font-sans focus:outline-none focus:ring-1 focus:ring-deep-ink"
                  autoFocus
                />
              </div>

              {/* Language List */}
              <div className="flex-1 overflow-y-auto space-y-1.5 pr-1 min-h-0">
                {filteredLanguages.map((lang) => {
                  const isSelected = currentLangCode === lang.code;
                  return (
                    <button
                      key={lang.code}
                      onClick={() => {
                        i18n.changeLanguage(lang.code);
                        localStorage.setItem('i18nextLng', lang.code);
                        setShowLangOverlay(false);
                      }}
                      className={`w-full flex items-center justify-between p-2.5 rounded-xl border text-left transition-all cursor-pointer select-none ${isSelected
                        ? 'bg-deep-ink text-white border-deep-ink shadow-2xs'
                        : 'bg-canvas hover:bg-canvas/80 border-onyx/5 text-deep-ink'
                        }`}
                    >
                      <div className="flex items-center gap-2.5 min-w-0">
                        <span className="text-caption font-semibold shrink-0">{lang.shortCode}</span>
                        <div className="min-w-0">
                          <div className="font-sans font-semibold text-caption truncate leading-snug">
                            {lang.nativeName}
                          </div>
                          <div
                            className={`text-[10px] font-sans truncate ${isSelected ? 'text-white/70' : 'text-slate'
                              }`}
                          >
                            {lang.name}
                          </div>
                        </div>
                      </div>

                      <div className="flex items-center gap-1 shrink-0 ml-2">
                        {lang.coverage === '100%' && (
                          <span
                            className={`text-[9px] font-mono px-1.5 py-0.5 rounded-full font-semibold ${isSelected
                              ? 'bg-hi-yellow text-charcoal'
                              : 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300'
                              }`}
                          >
                            {t('language.full')}
                          </span>
                        )}
                        {isSelected && <Check className="w-3.5 h-3.5 text-hi-yellow" />}
                      </div>
                    </button>
                  );
                })}
              </div>
            </div>

            {/* Footer */}
            <div className="pt-3 border-t border-onyx/10 flex items-center justify-between text-[11px] font-mono text-slate shrink-0">
              <span>ActonOS</span>
              <button
                onClick={() => setShowLangOverlay(false)}
                className="text-caption font-sans font-medium text-deep-ink hover:underline cursor-pointer"
              >
                {t('language.close')}
              </button>
            </div>
          </div>
        )}
      </aside>
    </>
  );
}
