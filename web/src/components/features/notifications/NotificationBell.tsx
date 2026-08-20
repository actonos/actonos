import { useState, useRef, useEffect, useTransition } from 'react';
import {
  Bell,
  CheckCheck,
  ShieldAlert,
  AlertTriangle,
  AlertCircle,
  Info,
  CheckCircle2,
  ExternalLink,
  ChevronRight,
  Sparkles,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { api } from '@/lib/api';
import { useRealtime } from '@/components/providers/RealtimeProvider';
import { useWebNotifications } from '@/lib/useWebNotifications';
import type { NotificationItem, NotificationType } from '@/lib/types';
import type { NavTab } from '@/components/layout/Sidebar';

interface NotificationBellProps {
  onNavigateTab?: (tab: NavTab) => void;
}

export function NotificationBell({ onNavigateTab }: NotificationBellProps) {
  const { t } = useTranslation(['notifications', 'common']);
  const { snapshot } = useRealtime();
  const { permission, requestPermission } = useWebNotifications();

  const [isOpen, setIsOpen] = useState(false);
  const [notifications, setNotifications] = useState<NotificationItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [, startTransition] = useTransition();
  const dropdownRef = useRef<HTMLDivElement>(null);

  const unreadCount = snapshot?.notifications_unread ?? 0;

  const loadRecent = async () => {
    try {
      setLoading(true);
      const res = await api.listNotifications({ page: 1, limit: 7 });
      setNotifications(res.notifications || []);
    } catch (err) {
      console.warn('Failed to load recent notifications', err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (isOpen) {
      loadRecent();
    }
  }, [isOpen, snapshot?.notifications_unread]);

  // Click outside to close dropdown
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
        setIsOpen(false);
      }
    };
    if (isOpen) {
      document.addEventListener('mousedown', handleClickOutside);
    }
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [isOpen]);

  const handleMarkAllRead = async () => {
    try {
      await api.markNotificationRead(undefined, true);
      startTransition(() => {
        setNotifications((prev) => prev.map((n) => ({ ...n, is_read: true })));
      });
    } catch (err) {
      console.warn('Failed to mark all as read', err);
    }
  };

  const handleItemClick = async (item: NotificationItem) => {
    if (!item.is_read) {
      try {
        await api.markNotificationRead(item.id);
        startTransition(() => {
          setNotifications((prev) =>
            prev.map((n) => (n.id === item.id ? { ...n, is_read: true } : n))
          );
        });
      } catch { }
    }
    setIsOpen(false);
    if (item.link) {
      const target = item.link.replace(/^#\/?/, '').replace(/^\//, '') as NavTab;
      if (onNavigateTab) {
        onNavigateTab(target);
      } else {
        window.location.hash = `#/${target}`;
      }
    }
  };

  const handleViewAll = () => {
    setIsOpen(false);
    if (onNavigateTab) {
      onNavigateTab('notifications');
    } else {
      window.location.hash = '#/notifications';
    }
  };

  const formatRelativeTime = (iso: string) => {
    try {
      const diff = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
      if (diff < 60) return `${Math.max(1, diff)}s`;
      if (diff < 3600) return `${Math.floor(diff / 60)}m`;
      if (diff < 86400) return `${Math.floor(diff / 3600)}h`;
      return `${Math.floor(diff / 86400)}d`;
    } catch {
      return '';
    }
  };

  const getIcon = (type: NotificationType) => {
    switch (type) {
      case 'approval':
        return <ShieldAlert className="w-4 h-4 text-amber-600 dark:text-amber-400" />;
      case 'error':
        return <AlertTriangle className="w-4 h-4 text-crimson dark:text-rose-400" />;
      case 'warning':
        return <AlertCircle className="w-4 h-4 text-amber-500" />;
      case 'success':
        return <CheckCircle2 className="w-4 h-4 text-emerald-600 dark:text-emerald-400" />;
      default:
        return <Info className="w-4 h-4 text-electric-cyan" />;
    }
  };

  return (
    <div className="relative inline-block" ref={dropdownRef}>
      {/* Bell Trigger Button */}
      <button
        type="button"
        onClick={() => setIsOpen((prev) => !prev)}
        className={`relative p-2 rounded-full border transition-all cursor-pointer ${isOpen
            ? 'bg-soft-meadow border-onyx/20 text-deep-ink shadow-xs'
            : 'bg-soft-meadow border-onyx/10 text-slate hover:bg-canvas hover:text-deep-ink'
          }`}
        aria-label={t('title', 'Notifications')}
        title={t('title', 'Notifications')}
      >
        <Bell className="w-4 h-4" />
        {unreadCount > 0 && (
          <span className="absolute -top-1 -right-1 flex h-4 min-w-4 px-1 items-center justify-center rounded-full bg-red-500 text-[10px] font-mono font-bold text-white shadow-xs animate-pulse">
            {unreadCount > 99 ? '99+' : unreadCount}
          </span>
        )}
      </button>

      {/* Dropdown Popup */}
      {isOpen && (
        <div className="absolute right-0 mt-2 w-80 sm:w-96 rounded-2xl border border-onyx/10 bg-canvas/95 backdrop-blur-xl shadow-xl z-50 overflow-hidden animate-in fade-in zoom-in-95 duration-150">
          {/* Header */}
          <div className="flex items-center justify-between px-4 py-3 border-b border-onyx/10 bg-soft-meadow/50">
            <div className="flex items-center gap-2">
              <span className="font-serif font-bold text-body-sm text-deep-ink">
                {t('title', 'Notifications')}
              </span>
              {unreadCount > 0 && (
                <span className="px-2 py-0.5 rounded-full bg-red-500/10 text-crimson text-[11px] font-mono font-semibold">
                  {unreadCount}
                </span>
              )}
            </div>

            {unreadCount > 0 && (
              <button
                type="button"
                onClick={handleMarkAllRead}
                className="flex items-center gap-1 text-caption text-slate hover:text-deep-ink transition-colors cursor-pointer"
                title={t('markAllRead', 'Mark all as read')}
              >
                <CheckCheck className="w-3.5 h-3.5" />
                <span>{t('markAllRead', 'Mark all read')}</span>
              </button>
            )}
          </div>

          {/* Browser Push Permission Banner */}
          {permission === 'default' && (
            <div className="p-3 bg-hi-yellow/10 border-b border-hi-yellow/20 flex items-center justify-between gap-2 text-caption">
              <div className="flex items-center gap-2 text-deep-ink">
                <Sparkles className="w-4 h-4 text-amber-600 shrink-0" />
                <span className="line-clamp-1">{t('enableDesktop', 'Enable desktop notifications')}</span>
              </div>
              <button
                type="button"
                onClick={requestPermission}
                className="px-2.5 py-1 rounded-lg bg-deep-ink text-white hover:bg-onyx text-caption font-semibold shrink-0 transition-colors cursor-pointer"
              >
                {t('actions.confirm', 'Allow')}
              </button>
            </div>
          )}

          {/* List Content */}
          <div className="max-h-80 overflow-y-auto divide-y divide-onyx/5">
            {loading && notifications.length === 0 ? (
              <div className="py-8 flex items-center justify-center text-caption text-slate">
                <div className="w-4 h-4 border-2 border-deep-ink border-t-transparent rounded-full animate-spin mr-2" />
                <span>Loading...</span>
              </div>
            ) : notifications.length === 0 ? (
              <div className="py-8 px-4 text-center">
                <p className="text-body-sm font-medium text-deep-ink mb-1">
                  {t('emptyTitle', 'No notifications yet')}
                </p>
                <p className="text-caption text-slate">
                  {t('emptyDesc', 'New mission alerts and approval prompts will appear here.')}
                </p>
              </div>
            ) : (
              notifications.map((notif) => (
                <div
                  key={notif.id}
                  onClick={() => handleItemClick(notif)}
                  className={`p-3.5 flex items-start gap-3 hover:bg-soft-meadow/60 transition-colors cursor-pointer ${!notif.is_read ? 'bg-soft-meadow/25' : ''
                    }`}
                >
                  <div className="mt-0.5 p-1.5 rounded-xl bg-canvas border border-onyx/10 shrink-0 shadow-2xs">
                    {getIcon(notif.type)}
                  </div>

                  <div className="flex-1 min-w-0">
                    <div className="flex items-center justify-between gap-1 mb-0.5">
                      <h4
                        className={`text-caption font-medium truncate ${!notif.is_read ? 'text-deep-ink font-semibold' : 'text-slate'
                          }`}
                      >
                        {notif.title}
                      </h4>
                      <span className="text-[10px] font-mono text-slate/70 shrink-0">
                        {formatRelativeTime(notif.created_at)}
                      </span>
                    </div>
                    <p className="text-caption text-slate/90 line-clamp-2 leading-relaxed">
                      {notif.message}
                    </p>
                    {notif.link && (
                      <div className="mt-1 flex items-center gap-1 text-[11px] text-electric-cyan font-mono font-medium">
                        <span>{t('actions.goToLink', 'View Target')}</span>
                        <ExternalLink className="w-2.5 h-2.5" />
                      </div>
                    )}
                  </div>

                  {!notif.is_read && (
                    <span className="w-2 h-2 rounded-full bg-electric-cyan shrink-0 mt-2 self-start" />
                  )}
                </div>
              ))
            )}
          </div>

          {/* Footer */}
          <div className="p-2 border-t border-onyx/10 bg-soft-meadow/40 text-center">
            <button
              type="button"
              onClick={handleViewAll}
              className="w-full py-1.5 px-3 rounded-xl hover:bg-canvas text-caption font-medium text-slate hover:text-deep-ink transition-colors flex items-center justify-center gap-1 cursor-pointer"
            >
              <span>{t('viewAll', 'View all notifications')}</span>
              <ChevronRight className="w-3.5 h-3.5" />
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
