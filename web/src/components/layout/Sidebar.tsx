import { useTranslation } from 'react-i18next';
import { LanguageSwitcher } from '@/components/ui/LanguageSwitcher';
import {
  Bot,
  MessageSquare,
  Wrench,
  Folder,
  Layers,
  Sliders,
  ChevronLeft,
  ChevronRight,
  X,
} from 'lucide-react';

export type NavTab = 'agents' | 'chat' | 'tools' | 'workspace' | 'integrations' | 'settings';

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
  const { t } = useTranslation('nav');

  const navItems: { id: NavTab; label: string; desc: string; icon: React.ElementType }[] = [
    { id: 'agents', label: t('links.agents', 'Agents'), desc: 'Autonomous Swarms', icon: Bot },
    { id: 'chat', label: t('links.chat', 'Chat Canvas'), desc: 'ReAct Reasoning', icon: MessageSquare },
    { id: 'tools', label: t('links.toolHub', 'Tool Hub'), desc: 'Native & MCP Host', icon: Wrench },
    { id: 'workspace', label: t('links.workspace', 'Workspace'), desc: 'Sandboxed Files', icon: Folder },
    { id: 'integrations', label: t('links.integrations', 'Integrations'), desc: 'SaaS & Channels', icon: Layers },
    { id: 'settings', label: t('links.settings', 'Settings'), desc: 'Keys & Hardware', icon: Sliders },
  ];

  const handleSelect = (tab: NavTab) => {
    onSelectTab(tab);
    onCloseMobile();
  };

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
        className={`fixed top-0 left-0 bottom-0 z-50 bg-soft-meadow border-r border-onyx/10 flex flex-col justify-between transition-all duration-200 ease-in-out ${collapsed ? 'w-20' : 'w-64'
          } ${mobileOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0'
          }`}
      >
        {/* Top: Logo & Title */}
        <div>
          <div className="h-16 px-4 flex items-center justify-between border-b border-onyx/10">
            <div
              className="flex items-center gap-3 cursor-pointer select-none overflow-hidden"
              onClick={() => handleSelect('agents')}
            >
              <img
                src="/actonos_logo.png"
                alt="ActonOS"
                className="h-8 w-auto object-contain shrink-0"
              />
            </div>

            {/* Mobile close button */}
            <button
              onClick={onCloseMobile}
              className="lg:hidden p-1.5 rounded-full hover:bg-black/5 text-deep-ink"
            >
              <X className="w-5 h-5" />
            </button>
          </div>

          {/* Navigation Links List */}
          <nav className="p-3 space-y-1.5">
            {navItems.map((item) => {
              const Icon = item.icon;
              const isActive = activeTab === item.id;
              return (
                <button
                  key={item.id}
                  onClick={() => handleSelect(item.id)}
                  title={collapsed ? `${item.label} — ${item.desc}` : undefined}
                  className={`w-full flex items-center gap-3 px-3.5 py-3 rounded-[16px] transition-all cursor-pointer text-left group select-none ${isActive
                      ? 'bg-deep-ink text-white font-semibold shadow-xs'
                      : 'text-deep-ink hover:bg-canvas hover:text-deep-ink'
                    }`}
                >
                  <div
                    className={`w-8 h-8 rounded-full flex items-center justify-center shrink-0 transition-colors ${isActive
                        ? 'bg-white/15 text-hi-yellow'
                        : 'bg-canvas text-deep-ink group-hover:bg-white'
                      }`}
                  >
                    <Icon className="w-4 h-4" />
                  </div>

                  {!collapsed && (
                    <div className="flex flex-col truncate">
                      <span className="text-body-sm font-medium leading-snug truncate">
                        {item.label}
                      </span>
                      <span
                        className={`text-[11px] truncate ${isActive ? 'text-white/70' : 'text-slate'
                          }`}
                      >
                        {item.desc}
                      </span>
                    </div>
                  )}
                </button>
              );
            })}
          </nav>
        </div>

        {/* Bottom: System Status, Language & Collapse button */}
        <div className="p-3 border-t border-onyx/10 space-y-2 bg-soft-meadow">
          {/* Live Status Pill */}
          {!collapsed ? (
            <div className="p-2.5 rounded-[14px] bg-canvas border border-onyx/5 flex items-center justify-between text-caption">
              <div className="flex items-center gap-2">
                <span className="relative flex h-2 w-2">
                  <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                  <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
                </span>
                <span className="font-mono font-medium text-deep-ink">System Online</span>
              </div>
              <span className="text-slate font-mono text-[10px]">Docker HAL</span>
            </div>
          ) : (
            <div className="flex justify-center py-1" title="System Online">
              <span className="relative flex h-2.5 w-2.5">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-emerald-500"></span>
              </span>
            </div>
          )}

          {/* Language Switcher & Collapse Toggle */}
          <div className={`flex items-center ${collapsed ? 'flex-col gap-2' : 'justify-between gap-2'}`}>
            <LanguageSwitcher />

            <button
              onClick={onToggleCollapse}
              className="hidden lg:flex items-center justify-center p-2 rounded-full hover:bg-canvas text-slate hover:text-deep-ink transition-colors border border-onyx/10 cursor-pointer"
              title={collapsed ? 'Expand Sidebar' : 'Collapse Sidebar'}
            >
              {collapsed ? <ChevronRight className="w-4 h-4" /> : <ChevronLeft className="w-4 h-4" />}
            </button>
          </div>
        </div>
      </aside>
    </>
  );
}
