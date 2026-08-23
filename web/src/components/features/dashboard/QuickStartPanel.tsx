import { useTranslation } from 'react-i18next';
import { Bot, CheckCircle2, Circle, Key, MessageSquare, Radio, Sparkles, X } from 'lucide-react';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import type { NavTab } from '@/components/layout/Sidebar';

export interface QuickStartPanelProps {
  completedSteps: Record<string, boolean>;
  onToggleStep: (stepID: string) => void;
  onDismiss: () => void;
  onNavigate: (tab: NavTab) => void;
}

export function QuickStartPanel({ completedSteps, onToggleStep, onDismiss, onNavigate }: QuickStartPanelProps) {
  const { t } = useTranslation('dashboard');
  const steps = [
    { id: 'keys', icon: Key, tab: 'settings' as const },
    { id: 'channel', icon: Radio, tab: 'plugins' as const },
    { id: 'agent', icon: Bot, tab: 'agents' as const },
    { id: 'chat', icon: MessageSquare, tab: 'chat' as const },
    { id: 'skills', icon: Sparkles, tab: 'skills' as const },
  ].map((step) => ({
    ...step,
    title: t(`quickstart.steps.${step.id}.title`),
    description: t(`quickstart.steps.${step.id}.desc`),
    action: t(`quickstart.steps.${step.id}.action`),
  }));
  const completedCount = steps.filter((step) => completedSteps[step.id]).length;
  const progress = Math.round((completedCount / steps.length) * 100);

  return (
    <Card className="mb-8 overflow-hidden border-2 border-deep-ink/15 bg-canvas/95">
      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <div className="flex items-center gap-2">
            <div className="flex h-8 w-8 items-center justify-center rounded-full bg-deep-ink text-hi-yellow">
              <Sparkles className="h-4 w-4" />
            </div>
            <h2 className="font-serif text-heading-sm font-semibold text-deep-ink">{t('quickstart.title')}</h2>
          </div>
          <p className="mt-1 text-body-sm text-slate">{t('quickstart.subtitle')}</p>
        </div>
        <div className="flex items-center gap-3">
          <div className="min-w-32">
            <p className="text-right text-caption font-semibold text-deep-ink">
              {t('quickstart.progress', { completed: completedCount, total: steps.length })}
            </p>
            <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-onyx/10">
              <div className="h-full rounded-full bg-deep-ink" style={{ width: `${progress}%` }} />
            </div>
          </div>
          <button type="button" onClick={onDismiss} aria-label={t('quickstart.dismiss')} title={t('quickstart.dismiss')} className="flex h-10 w-10 items-center justify-center rounded-full text-slate hover:bg-soft-meadow hover:text-deep-ink">
            <X className="h-4 w-4" />
          </button>
        </div>
      </div>
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-5">
        {steps.map((step, index) => {
          const Icon = step.icon;
          const done = Boolean(completedSteps[step.id]);
          return (
            <article key={step.id} className="flex flex-col justify-between rounded-[18px] border border-onyx/10 bg-soft-meadow p-4">
              <div>
                <div className="mb-2 flex items-center justify-between">
                  <span className="text-caption font-semibold text-slate">{String(index + 1).padStart(2, '0')}</span>
                  <button type="button" onClick={() => onToggleStep(step.id)} aria-label={done ? t('quickstart.markIncomplete') : t('quickstart.markComplete')} className="flex h-9 w-9 items-center justify-center rounded-full hover:bg-canvas">
                    {done ? <CheckCircle2 className="h-4 w-4 text-status-success" /> : <Circle className="h-4 w-4 text-slate" />}
                  </button>
                </div>
                <div className="mb-1 flex items-center gap-2">
                  <Icon className="h-4 w-4 shrink-0 text-deep-ink" />
                  <h3 className={`truncate text-body-sm font-semibold text-deep-ink ${done ? 'line-through opacity-70' : ''}`}>{step.title}</h3>
                </div>
                <p className="mb-3 line-clamp-2 text-caption text-slate">{step.description}</p>
              </div>
              <Button variant={done ? 'ghost' : 'primary'} size="sm" onClick={() => onNavigate(step.tab)} className="w-full">
                {step.action}
              </Button>
            </article>
          );
        })}
      </div>
    </Card>
  );
}
