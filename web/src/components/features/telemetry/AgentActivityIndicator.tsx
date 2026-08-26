import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Bot, Calendar, HeartPulse, MessageSquare, Radio } from 'lucide-react';
import { useRealtime } from '@/components/providers/RealtimeProvider';
import type { NavTab } from '@/components/layout/Sidebar';
import type { AgentRun } from '@/lib/types';

export interface AgentActivityIndicatorProps {
  onNavigateTab?: (tab: NavTab) => void;
  onOpenChat?: (agentID: string) => void;
}

function truncate(text: string, max: number) {
  const trimmed = text.trim();
  if (trimmed.length <= max) return trimmed;
  return `${trimmed.slice(0, max - 1)}…`;
}

export function AgentActivityIndicator({ onNavigateTab, onOpenChat }: AgentActivityIndicatorProps) {
  const { t } = useTranslation('nav');
  const { snapshot } = useRealtime();
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

  const running = useMemo(
    () => (snapshot?.runs || []).filter((run) => run.status === 'running'),
    [snapshot?.runs],
  );
  const count = running.length;
  const top = running[0];

  useEffect(() => {
    if (!open) return;
    const onPointer = (event: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false);
    };
    document.addEventListener('mousedown', onPointer);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onPointer);
      document.removeEventListener('keydown', onKey);
    };
  }, [open]);

  const label = count === 0
    ? t('activity.idle')
    : count === 1
      ? t('activity.runningOne', {
        name: top?.agent_name || top?.agent_id || '',
        task: truncate(top?.goal || '', 40),
      })
      : t('activity.runningMany', { count });

  const openRun = (run: AgentRun) => {
    setOpen(false);
    window.location.hash = `/operations?view=feed&run=${encodeURIComponent(run.id)}`;
    onNavigateTab?.('operations');
  };

  return (
    <div ref={rootRef} className="relative">
      <button
        type="button"
        className="flex min-h-9 min-w-11 items-center gap-2 rounded-full border border-onyx/10 bg-soft-meadow px-3 text-caption text-deep-ink transition-colors hover:bg-canvas"
        aria-label={label}
        aria-expanded={open}
        aria-haspopup="dialog"
        onClick={() => setOpen((prev) => !prev)}
      >
        <span className="relative flex h-4 w-4 items-center justify-center">
          <Bot className="h-4 w-4 text-slate" />
          {count > 0 && (
            <span className="absolute -right-0.5 -top-0.5 h-2 w-2 rounded-full bg-emerald-500 motion-safe:animate-pulse" />
          )}
        </span>
        <span className="hidden md:inline max-w-[220px] truncate font-sans">{label}</span>
        <span className="md:hidden font-mono text-[11px]" aria-hidden="true">{count}</span>
        <span className="sr-only md:hidden">{label}</span>
      </button>
      <div aria-live="polite" className="sr-only">
        {label}
      </div>
      {open && (
        <div
          role="dialog"
          className="absolute right-0 top-full z-40 mt-2 w-80 rounded-[24px] border border-onyx/10 bg-canvas p-3"
        >
          {count === 0 ? (
            <p className="px-2 py-3 text-caption text-slate">{t('activity.empty')}</p>
          ) : (
            <ul className="space-y-1">
              {running.map((run) => (
                <li key={run.id}>
                  <div className="flex items-start gap-1">
                    <button
                      type="button"
                      className="min-w-0 flex-1 rounded-[18px] px-3 py-2 text-left hover:bg-soft-meadow"
                      onClick={() => openRun(run)}
                    >
                      <p className="truncate text-caption font-semibold text-deep-ink">
                        {run.agent_name || run.agent_id}
                      </p>
                      <p className="truncate text-[11px] text-slate">{run.goal}</p>
                    </button>
                    {run.source === 'chat' || run.source === 'stream' ? (
                      <button
                        type="button"
                        className="mt-1 rounded-full p-2 hover:bg-soft-meadow"
                        aria-label={t('activity.openChat')}
                        onClick={() => {
                          setOpen(false);
                          onOpenChat?.(run.agent_id);
                        }}
                      >
                        <MessageSquare className="h-3.5 w-3.5" />
                      </button>
                    ) : null}
                    {run.source === 'cron' ? (
                      <button
                        type="button"
                        className="mt-1 rounded-full p-2 hover:bg-soft-meadow"
                        aria-label={t('activity.openAutomations')}
                        onClick={() => {
                          setOpen(false);
                          onNavigateTab?.('automations');
                        }}
                      >
                        <Calendar className="h-3.5 w-3.5" />
                      </button>
                    ) : null}
                    {run.source === 'heartbeat' ? (
                      <button
                        type="button"
                        className="mt-1 rounded-full p-2 hover:bg-soft-meadow"
                        aria-label={t('activity.openMissions')}
                        onClick={() => {
                          setOpen(false);
                          onNavigateTab?.('missions');
                        }}
                      >
                        <HeartPulse className="h-3.5 w-3.5" />
                      </button>
                    ) : null}
                    {run.source === 'channel' ? (
                      <Radio className="mt-2.5 mr-2 h-3.5 w-3.5 text-slate" />
                    ) : null}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
