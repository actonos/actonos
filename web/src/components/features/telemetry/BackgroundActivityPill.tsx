import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Zap, ArrowRight, X } from 'lucide-react';
import type { NavTab } from '@/components/layout/Sidebar';

export interface BackgroundActivityPillProps {
  onNavigateTab?: (tab: NavTab) => void;
}

export function BackgroundActivityPill({ onNavigateTab }: BackgroundActivityPillProps) {
  const { t } = useTranslation('common');
  const [activeTasks, setActiveTasks] = useState(0);
  const [dismissed, setDismissed] = useState(false);

  useEffect(() => {
    const handleBackendEvent = (e: Event) => {
      const customEvent = e as CustomEvent;
      const eventType = customEvent.detail?.type;

      if (eventType === 'task.created' || eventType === 'agent.reasoning' || eventType === 'skill.progress') {
        setActiveTasks((prev) => prev + 1);
        setDismissed(false);
      } else if (eventType === 'task.finished' || eventType === 'task.cancelled' || eventType === 'task.failed') {
        setActiveTasks((prev) => Math.max(0, prev - 1));
      }
    };

    window.addEventListener('actonos:backend-event', handleBackendEvent);
    return () => window.removeEventListener('actonos:backend-event', handleBackendEvent);
  }, []);

  if (activeTasks === 0 || dismissed) return null;

  return (
    <div className="fixed bottom-5 right-5 z-40 animate-slide-in">
      <div className="flex items-center gap-3 px-3.5 py-2 bg-deep-ink text-white rounded-full shadow-lg border border-onyx/20 backdrop-blur-md">
        {/* Animated Pulse Icon */}
        <div className="relative flex items-center justify-center">
          <Zap className="w-4 h-4 text-hi-yellow animate-pulse" />
        </div>

        {/* Text readout */}
        <span className="text-caption font-sans font-medium">
          {t('backgroundActivity.tasksRunning', { count: activeTasks })}
        </span>

        {/* Quick Navigate Button */}
        {onNavigateTab && (
          <button
            type="button"
            onClick={() => onNavigateTab('operations')}
            className="px-2 py-0.5 rounded-full bg-white/15 hover:bg-white/25 text-[11px] font-sans font-semibold text-white transition-colors flex items-center gap-1 cursor-pointer"
          >
            <span>{t('backgroundActivity.viewOperations', 'View')}</span>
            <ArrowRight className="w-3 h-3" />
          </button>
        )}

        {/* Dismiss Button */}
        <button
          type="button"
          onClick={() => setDismissed(true)}
          className="text-white/60 hover:text-white transition-colors p-0.5 rounded-full cursor-pointer"
          aria-label="Dismiss"
        >
          <X className="w-3.5 h-3.5" />
        </button>
      </div>
    </div>
  );
}
