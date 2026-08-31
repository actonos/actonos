import { useState, useEffect, useTransition, useCallback } from 'react';
import {
  Bell,
  CheckCheck,
  Trash2,
  RefreshCw,
  Search,
  ShieldAlert,
  AlertTriangle,
  AlertCircle,
  Info,
  CheckCircle2,
  ExternalLink,
  Filter,
  Sparkles,
  ChevronLeft,
  ChevronRight,
  ShieldCheck,
  Send,
  Zap,
  Moon,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { api } from '@/lib/api';
import { useWebNotifications } from '@/lib/useWebNotifications';
import { useRealtime } from '@/components/providers/RealtimeProvider';
import { ConfirmModal } from '@/components/ui/ConfirmModal';
import { PageHeader } from '@/components/ui/PageHeader';
import { EmptyState } from '@/components/ui/EmptyState';
import { NotificationPreferencesModal } from './components/NotificationPreferencesModal';
import type { NotificationItem, NotificationType } from '@/lib/types';
import type { NavTab } from '@/components/layout/Sidebar';

interface NotificationsPageProps {
  onNavigateTab?: (tab: NavTab) => void;
}

export function NotificationsPage({ onNavigateTab }: NotificationsPageProps) {
  const { t } = useTranslation(['notifications', 'common']);
  const { permission, isPushSubscribed, requestPermission, testPush } = useWebNotifications();
  const { snapshot } = useRealtime();

  const [notifications, setNotifications] = useState<NotificationItem[]>([]);
  const [total, setTotal] = useState(0);
  const [unreadCount, setUnreadCount] = useState(0);
  const [page, setPage] = useState(1);
  const [limit] = useState(15);
  const [selectedType, setSelectedType] = useState<string>('all');
  const [unreadOnly, setUnreadOnly] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [loading, setLoading] = useState(true);
  const [showClearModal, setShowClearModal] = useState(false);
  const [isPreferencesOpen, setIsPreferencesOpen] = useState(false);
  const [testingPush, setTestingPush] = useState(false);
  const [pushStatusMessage, setPushStatusMessage] = useState<string | null>(null);
  const [, startTransition] = useTransition();

  const loadNotifications = useCallback(async () => {
    try {
      setLoading(true);
      const res = await api.listNotifications({
        page,
        limit,
        type: selectedType === 'all' ? undefined : selectedType,
        unread_only: unreadOnly,
      });
      setNotifications(res.notifications || []);
      setTotal(res.total || 0);
      setUnreadCount(res.unread_count || 0);
    } catch (err) {
      console.warn('Failed to load notifications', err);
    } finally {
      setLoading(false);
    }
  }, [page, limit, selectedType, unreadOnly]);

  useEffect(() => {
    loadNotifications();
  }, [loadNotifications, snapshot?.notifications_unread]);

  const handleMarkAllRead = async () => {
    try {
      await api.markNotificationRead(undefined, true);
      startTransition(() => {
        setNotifications((prev) => prev.map((n) => ({ ...n, is_read: true })));
        setUnreadCount(0);
      });
    } catch (err) {
      console.warn('Failed to mark all as read', err);
    }
  };

  const handleMarkSingleRead = async (id: string) => {
    try {
      await api.markNotificationRead(id);
      startTransition(() => {
        setNotifications((prev) =>
          prev.map((n) => (n.id === id ? { ...n, is_read: true } : n))
        );
        setUnreadCount((c) => Math.max(0, c - 1));
      });
    } catch (err) {
      console.warn('Failed to mark as read', err);
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await api.deleteNotification(id);
      startTransition(() => {
        setNotifications((prev) => prev.filter((n) => n.id !== id));
        setTotal((t) => Math.max(0, t - 1));
      });
    } catch (err) {
      console.warn('Failed to delete notification', err);
    }
  };

  const handleClearAll = async () => {
    try {
      await api.clearAllNotifications();
      setShowClearModal(false);
      startTransition(() => {
        setNotifications([]);
        setTotal(0);
        setUnreadCount(0);
      });
    } catch (err) {
      console.warn('Failed to clear notifications', err);
    }
  };

  const handleTestPush = async () => {
    try {
      setTestingPush(true);
      await testPush({
        title: t('testNotificationTitle', 'ActonOS Background Alert'),
        message: t('testNotificationBody', 'Service Worker background push is working properly!'),
        link: '/notifications',
      });
      setPushStatusMessage(t('toast.testSent', 'Test push notification dispatched!'));
      setTimeout(() => setPushStatusMessage(null), 4000);
      loadNotifications();
    } catch (err) {
      console.warn('Failed to send test push', err);
    } finally {
      setTestingPush(false);
    }
  };

  const handleNavigate = (link?: string) => {
    if (!link) return;
    const target = link.replace(/^#\/?/, '').replace(/^\//, '') as NavTab;
    if (onNavigateTab) {
      onNavigateTab(target);
    } else {
      window.location.hash = `#/${target}`;
    }
  };

  const filteredNotifications = notifications.filter((item) => {
    if (!searchQuery.trim()) return true;
    const q = searchQuery.toLowerCase();
    return (
      item.title.toLowerCase().includes(q) ||
      item.message.toLowerCase().includes(q) ||
      item.category.toLowerCase().includes(q)
    );
  });

  const getIcon = (type: NotificationType) => {
    switch (type) {
      case 'approval':
        return <ShieldAlert className="w-5 h-5 text-amber-600 dark:text-amber-400" />;
      case 'error':
        return <AlertTriangle className="w-5 h-5 text-crimson dark:text-rose-400" />;
      case 'warning':
        return <AlertCircle className="w-5 h-5 text-amber-500" />;
      case 'success':
        return <CheckCircle2 className="w-5 h-5 text-emerald-600 dark:text-emerald-400" />;
      default:
        return <Info className="w-5 h-5 text-electric-cyan" />;
    }
  };

  const totalPages = Math.ceil(total / limit) || 1;
  const startItem = total === 0 ? 0 : (page - 1) * limit + 1;
  const endItem = Math.min(page * limit, total);

  // Metric counts
  const approvalCount = notifications.filter((n) => n.type === 'approval').length;
  const errorCount = notifications.filter((n) => n.type === 'error' || n.type === 'warning').length;

  return (
    <div className="space-y-6 max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
      {/* Page Header */}
      <PageHeader
        title={t('title', 'Notifications')}
        description={t('subtitle', 'Realtime alerts, task approvals, system failures, and autonomous mission updates.')}
        actions={
          <div className="flex flex-wrap items-center gap-2.5">
            {permission === 'default' ? (
              <button
                type="button"
                onClick={requestPermission}
                className="flex items-center gap-2 px-3.5 py-2 rounded-xl bg-hi-yellow/15 text-deep-ink border border-hi-yellow/30 hover:bg-hi-yellow/25 transition-colors text-caption font-medium cursor-pointer shadow-2xs"
              >
                <Sparkles className="w-4 h-4 text-amber-600" />
                <span>{t('enableDesktop', 'Enable Desktop Notifications')}</span>
              </button>
            ) : permission === 'granted' ? (
              <div className="flex items-center gap-2">
                <div className="hidden sm:flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border border-emerald-500/20 text-caption font-mono">
                  <ShieldCheck className="w-3.5 h-3.5" />
                  <span>
                    {isPushSubscribed
                      ? t('backgroundEnabled', 'Background Push (SW) Active')
                      : t('desktopEnabled', 'Desktop alerts active')}
                  </span>
                </div>
                <button
                  type="button"
                  onClick={handleTestPush}
                  disabled={testingPush}
                  className="flex items-center gap-1.5 px-3 py-2 rounded-xl bg-soft-meadow hover:bg-canvas text-slate hover:text-deep-ink border border-onyx/10 text-caption font-sans font-medium transition-colors cursor-pointer disabled:opacity-50"
                  title={t('sendTest', 'Send Test Push Notification')}
                >
                  <Send className={`w-3.5 h-3.5 text-electric-cyan ${testingPush ? 'animate-bounce' : ''}`} />
                  <span>{t('sendTest', 'Test Push')}</span>
                </button>
              </div>
            ) : null}

            {/* Smart Preferences & Quiet Hours Button */}
            <button
              type="button"
              onClick={() => setIsPreferencesOpen(true)}
              className="flex items-center gap-1.5 px-3 py-2 rounded-xl bg-soft-meadow hover:bg-canvas text-slate hover:text-deep-ink border border-onyx/10 text-caption font-sans font-medium transition-colors cursor-pointer"
              title={t('preferences.btnTitle', 'Quiet Hours & Smart Preferences')}
            >
              <Moon className="w-3.5 h-3.5 text-deep-ink" />
              <span>{t('preferences.btn', 'Preferences')}</span>
            </button>

            {unreadCount > 0 && (
              <button
                type="button"
                onClick={handleMarkAllRead}
                className="flex items-center gap-1.5 px-3.5 py-2 rounded-xl bg-soft-meadow hover:bg-canvas text-slate hover:text-deep-ink border border-onyx/10 text-caption font-sans font-medium transition-colors cursor-pointer"
              >
                <CheckCheck className="w-4 h-4" />
                <span>{t('markAllRead', 'Mark all read')}</span>
              </button>
            )}

            {total > 0 && (
              <button
                type="button"
                onClick={() => setShowClearModal(true)}
                className="flex items-center gap-1.5 px-3.5 py-2 rounded-xl bg-soft-meadow hover:bg-rose-50 dark:hover:bg-rose-950/30 text-slate hover:text-crimson border border-onyx/10 text-caption font-sans font-medium transition-colors cursor-pointer"
              >
                <Trash2 className="w-4 h-4" />
                <span>{t('clearAll', 'Clear history')}</span>
              </button>
            )}

            <button
              type="button"
              onClick={loadNotifications}
              className="p-2 rounded-xl bg-soft-meadow hover:bg-canvas text-slate hover:text-deep-ink border border-onyx/10 transition-colors cursor-pointer"
              title={t('actions.refresh', 'Refresh')}
            >
              <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
            </button>
          </div>
        }
      />

      {pushStatusMessage && (
        <div className="p-3 rounded-xl bg-electric-cyan/10 border border-electric-cyan/25 text-electric-cyan text-caption font-medium flex items-center gap-2 animate-in fade-in">
          <Zap className="w-4 h-4 shrink-0" />
          <span>{pushStatusMessage}</span>
        </div>
      )}

      {/* Metrics Banner */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
        <div className="p-4 rounded-2xl bg-canvas border border-onyx/10 shadow-2xs">
          <div className="flex items-center gap-2 text-slate text-caption mb-1">
            <Bell className="w-4 h-4 text-slate" />
            <span>{t('metrics.total', 'Total Notifications')}</span>
          </div>
          <p className="font-mono text-heading-md font-bold text-deep-ink">{total}</p>
        </div>

        <div className="p-4 rounded-2xl bg-canvas border border-onyx/10 shadow-2xs">
          <div className="flex items-center gap-2 text-slate text-caption mb-1">
            <Bell className="w-4 h-4 text-electric-cyan" />
            <span>{t('metrics.unread', 'Unread Alerts')}</span>
          </div>
          <p className="font-mono text-heading-md font-bold text-deep-ink">{unreadCount}</p>
        </div>

        <div className="p-4 rounded-2xl bg-canvas border border-onyx/10 shadow-2xs">
          <div className="flex items-center gap-2 text-slate text-caption mb-1">
            <ShieldAlert className="w-4 h-4 text-amber-500" />
            <span>{t('metrics.approvals', 'Pending Approvals')}</span>
          </div>
          <p className="font-mono text-heading-md font-bold text-amber-600 dark:text-amber-400">
            {approvalCount}
          </p>
        </div>

        <div className="p-4 rounded-2xl bg-canvas border border-onyx/10 shadow-2xs">
          <div className="flex items-center gap-2 text-slate text-caption mb-1">
            <AlertTriangle className="w-4 h-4 text-crimson" />
            <span>{t('metrics.errors', 'Critical Issues')}</span>
          </div>
          <p className="font-mono text-heading-md font-bold text-crimson">{errorCount}</p>
        </div>
      </div>

      {/* Filter & Search Toolbar */}
      <div className="flex flex-col sm:flex-row gap-3 items-stretch sm:items-center justify-between p-3 rounded-2xl bg-soft-meadow/60 border border-onyx/10">
        {/* Type Filter Pills */}
        <div className="flex flex-wrap items-center gap-1.5">
          {[
            { id: 'all', label: t('filters.all', 'All Types') },
            { id: 'approval', label: t('filters.approval', 'Approvals') },
            { id: 'error', label: t('filters.error', 'Errors & Alerts') },
            { id: 'warning', label: t('filters.warning', 'Warnings') },
            { id: 'info', label: t('filters.info', 'Info') },
            { id: 'success', label: t('filters.success', 'Success') },
          ].map((tab) => (
            <button
              key={tab.id}
              type="button"
              onClick={() => {
                setSelectedType(tab.id);
                setPage(1);
              }}
              className={`px-3 py-1.5 rounded-xl text-caption font-semibold transition-colors cursor-pointer ${
                selectedType === tab.id
                  ? 'bg-deep-ink text-white shadow-2xs'
                  : 'bg-canvas/80 text-slate hover:text-deep-ink border border-onyx/5'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {/* Search & Unread Toggle */}
        <div className="flex items-center gap-2">
          <div className="relative flex-1 sm:w-64">
            <Search className="w-3.5 h-3.5 absolute left-3 top-1/2 -translate-y-1/2 text-slate" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder={t('searchPlaceholder', 'Search notifications...')}
              className="w-full pl-8.5 pr-3 py-1.5 bg-canvas rounded-xl border border-onyx/10 text-caption text-deep-ink placeholder:text-slate/60 focus:outline-none focus:border-onyx/30"
            />
          </div>

          <button
            type="button"
            onClick={() => {
              setUnreadOnly((prev) => !prev);
              setPage(1);
            }}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-caption font-medium transition-colors cursor-pointer border ${
              unreadOnly
                ? 'bg-electric-cyan/15 border-electric-cyan text-deep-ink font-semibold'
                : 'bg-canvas border-onyx/10 text-slate hover:text-deep-ink'
            }`}
          >
            <Filter className="w-3.5 h-3.5" />
            <span>{t('filters.unreadOnly', 'Unread')}</span>
          </button>
        </div>
      </div>

      {/* Notifications List */}
      <div className="space-y-3">
        {loading && notifications.length === 0 ? (
          <div className="py-20 flex flex-col items-center justify-center text-slate">
            <div className="w-6 h-6 border-2 border-deep-ink border-t-transparent rounded-full animate-spin mb-3" />
            <p className="text-body-sm font-medium">Loading notifications...</p>
          </div>
        ) : filteredNotifications.length === 0 ? (
          <EmptyState
            title={t('emptyTitle', 'No notifications yet')}
            description={t('emptyDesc', 'You are completely caught up! New mission alerts and approval prompts will appear here.')}
          />
        ) : (
          filteredNotifications.map((notif) => (
            <div
              key={notif.id}
              className={`p-4 sm:p-5 rounded-2xl border transition-all duration-150 flex flex-col sm:flex-row items-start justify-between gap-4 shadow-2xs ${
                !notif.is_read
                  ? 'bg-canvas border-electric-cyan/30 ring-1 ring-electric-cyan/20'
                  : 'bg-canvas/70 border-onyx/10 hover:border-onyx/20'
              }`}
            >
              <div className="flex items-start gap-3.5 min-w-0 flex-1">
                <div className="p-2.5 rounded-xl bg-soft-meadow border border-onyx/10 shrink-0 shadow-2xs mt-0.5">
                  {getIcon(notif.type)}
                </div>

                <div className="space-y-1 min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <h3
                      className={`text-body-sm font-serif font-semibold tracking-tight ${
                        !notif.is_read ? 'text-deep-ink font-bold' : 'text-slate'
                      }`}
                    >
                      {notif.title}
                    </h3>
                    <span className="px-2 py-0.5 rounded-full bg-soft-meadow border border-onyx/10 text-[10px] font-mono text-slate uppercase">
                      {notif.category}
                    </span>
                    {!notif.is_read && (
                      <span className="px-2 py-0.5 rounded-full bg-electric-cyan/15 text-deep-ink text-[10px] font-mono font-bold">
                        NEW
                      </span>
                    )}
                  </div>

                  <p className="text-body-xs text-slate/90 leading-relaxed font-sans whitespace-pre-wrap">
                    {notif.message}
                  </p>

                  <div className="flex items-center gap-3 pt-1 text-[11px] font-mono text-slate/70">
                    <span>{new Date(notif.created_at).toLocaleString()}</span>
                  </div>
                </div>
              </div>

              {/* Action Buttons */}
              <div className="flex items-center gap-2 self-end sm:self-center shrink-0">
                {notif.link && (
                  <button
                    type="button"
                    onClick={() => handleNavigate(notif.link)}
                    className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-soft-meadow hover:bg-deep-ink hover:text-white text-deep-ink border border-onyx/10 text-caption font-medium transition-colors cursor-pointer"
                  >
                    <span>{t('actions.goToLink', 'View Target')}</span>
                    <ExternalLink className="w-3.5 h-3.5" />
                  </button>
                )}

                {!notif.is_read && (
                  <button
                    type="button"
                    onClick={() => handleMarkSingleRead(notif.id)}
                    className="p-2 rounded-xl bg-soft-meadow hover:bg-canvas text-slate hover:text-deep-ink border border-onyx/10 transition-colors cursor-pointer"
                    title={t('actions.markRead', 'Mark as read')}
                  >
                    <CheckCheck className="w-4 h-4" />
                  </button>
                )}

                <button
                  type="button"
                  onClick={() => handleDelete(notif.id)}
                  className="p-2 rounded-xl bg-soft-meadow hover:bg-rose-50 text-slate hover:text-crimson border border-onyx/10 transition-colors cursor-pointer"
                  title={t('actions.delete', 'Delete')}
                >
                  <Trash2 className="w-4 h-4" />
                </button>
              </div>
            </div>
          ))
        )}
      </div>

      {/* Pagination Footer */}
      {total > 0 && (
        <div className="flex flex-col sm:flex-row items-center justify-between gap-3 pt-4 border-t border-onyx/10 text-caption text-slate font-mono">
          <div>
            {t('pagination.showing', {
              start: startItem,
              end: endItem,
              total,
            })}
          </div>

          <div className="flex items-center gap-2">
            <button
              type="button"
              disabled={page <= 1}
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              className="flex items-center gap-1 px-3 py-1.5 rounded-xl bg-canvas border border-onyx/10 text-slate hover:text-deep-ink disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
            >
              <ChevronLeft className="w-3.5 h-3.5" />
              <span>{t('pagination.prev', 'Previous')}</span>
            </button>

            <span className="px-3 py-1 bg-soft-meadow rounded-xl border border-onyx/10 font-bold text-deep-ink">
              {t('pagination.page', { current: page, total: totalPages })}
            </span>

            <button
              type="button"
              disabled={page >= totalPages}
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              className="flex items-center gap-1 px-3 py-1.5 rounded-xl bg-canvas border border-onyx/10 text-slate hover:text-deep-ink disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
            >
              <span>{t('pagination.next', 'Next')}</span>
              <ChevronRight className="w-3.5 h-3.5" />
            </button>
          </div>
        </div>
      )}

      {/* Clear All Confirmation Modal */}
      <ConfirmModal
        isOpen={showClearModal}
        title={t('clearAll', 'Clear history')}
        description={t('clearAllConfirm', 'Are you sure you want to clear all notification history? This action cannot be undone.')}
        confirmLabel={t('actions.confirm', 'Clear All')}
        cancelLabel={t('actions.cancel', 'Cancel')}
        variant="danger"
        onConfirm={handleClearAll}
        onClose={() => setShowClearModal(false)}
      />

      {/* Smart Notification Preferences Modal */}
      <NotificationPreferencesModal
        isOpen={isPreferencesOpen}
        onClose={() => setIsPreferencesOpen(false)}
        onPreferencesUpdated={loadNotifications}
      />
    </div>
  );
}
