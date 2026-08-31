import { useTranslation } from 'react-i18next';
import { useRealtime } from '@/components/providers/RealtimeProvider';
import type { NavTab } from '@/components/layout/Sidebar';
import {
  LayoutDashboard,
  Target,
  MessageSquare,
  Bell,
  Menu,
} from 'lucide-react';

export interface MobileBottomNavProps {
  activeTab: NavTab;
  onSelectTab: (tab: NavTab) => void;
  onOpenSidebar: () => void;
}

export function MobileBottomNav({
  activeTab,
  onSelectTab,
  onOpenSidebar,
}: MobileBottomNavProps) {
  const { t } = useTranslation('nav');
  const { snapshot } = useRealtime();
  const unreadCount = snapshot?.notifications_unread ?? 0;
  const pendingApprovals = snapshot?.approvals?.length ?? 0;

  const navItems = [
    {
      id: 'dashboard' as NavTab,
      label: t('links.dashboard', 'Dashboard'),
      icon: LayoutDashboard,
    },
    {
      id: 'missions' as NavTab,
      label: t('links.missions', 'Missions'),
      icon: Target,
      badge: pendingApprovals > 0 ? pendingApprovals : undefined,
      badgeVariant: 'accent' as const,
    },
    {
      id: 'chat' as NavTab,
      label: t('links.chat', 'Chat'),
      icon: MessageSquare,
    },
    {
      id: 'notifications' as NavTab,
      label: t('links.notifications', 'Alerts'),
      icon: Bell,
      badge: unreadCount > 0 ? unreadCount : undefined,
      badgeVariant: 'info' as const,
    },
  ];

  return (
    <nav
      aria-label="Mobile Navigation"
      className="lg:hidden fixed bottom-0 left-0 right-0 z-40 bg-canvas/95 backdrop-blur-xl border-t border-onyx/10 px-2 py-1.5 flex items-center justify-around shadow-lg select-none pb-[calc(0.375rem+env(safe-area-inset-bottom))]"
    >
      {navItems.map((item) => {
        const Icon = item.icon;
        const isActive = activeTab === item.id;
        return (
          <button
            key={item.id}
            type="button"
            onClick={() => onSelectTab(item.id)}
            className={`flex flex-col items-center justify-center flex-1 py-1 px-1 rounded-xl transition-all relative ${
              isActive
                ? 'text-deep-ink font-semibold'
                : 'text-slate hover:text-deep-ink font-normal'
            }`}
          >
            <div className="relative">
              <div
                className={`p-1 rounded-lg transition-transform ${
                  isActive ? 'bg-soft-meadow scale-110 shadow-xs' : ''
                }`}
              >
                <Icon className={`w-5 h-5 ${isActive ? 'text-deep-ink stroke-[2.2]' : 'text-slate stroke-[1.8]'}`} />
              </div>
              {item.badge !== undefined && (
                <span
                  className={`absolute -top-1 -right-2 min-w-[16px] h-4 px-1 rounded-full text-[10px] font-bold flex items-center justify-center text-white ${
                    item.badgeVariant === 'accent' ? 'bg-accent-coral' : 'bg-blue-600'
                  }`}
                >
                  {item.badge > 99 ? '99+' : item.badge}
                </span>
              )}
            </div>
            <span className="text-[10px] tracking-tight mt-0.5 max-w-[64px] truncate">
              {item.label}
            </span>
          </button>
        );
      })}

      {/* More / Menu Drawer Trigger */}
      <button
        type="button"
        onClick={onOpenSidebar}
        className="flex flex-col items-center justify-center flex-1 py-1 px-1 rounded-xl text-slate hover:text-deep-ink transition-all"
      >
        <div className="p-1 rounded-lg">
          <Menu className="w-5 h-5 stroke-[1.8]" />
        </div>
        <span className="text-[10px] tracking-tight mt-0.5">
          {t('mobile.menu', 'Menu')}
        </span>
      </button>
    </nav>
  );
}
