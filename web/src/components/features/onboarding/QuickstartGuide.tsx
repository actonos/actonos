import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Button } from '@/components/ui/Button';
import {
  Key,
  Bot,
  Wrench,
  ShieldCheck,
  CheckCircle2,
  ArrowRight,
  Sparkles,
  ChevronUp,
  ChevronDown,
  X,
} from 'lucide-react';

import type { NavTab } from '@/components/layout/Sidebar';

export interface QuickstartGuideProps {
  onNavigateTab: (tab: NavTab) => void;
  onOpenChat: (agentID: string) => void;
}

export function QuickstartGuide({ onNavigateTab, onOpenChat }: QuickstartGuideProps) {
  const { t } = useTranslation('agents');
  const [collapsed, setCollapsed] = useState(false);
  const [dismissed, setDismissed] = useState(false);

  useEffect(() => {
    const isDismissed = localStorage.getItem('actonos_onboarding_dismissed') === 'true';
    if (isDismissed) {
      setDismissed(true);
    }
  }, []);

  const handleDismiss = () => {
    setDismissed(true);
    localStorage.setItem('actonos_onboarding_dismissed', 'true');
  };

  if (dismissed) {
    return null;
  }

  const steps = [
    {
      id: 1,
      icon: Key,
      title: t('onboarding.step1Title', 'Step 1: Configure LLM Provider'),
      desc: t('onboarding.step1Desc', 'Set your Anthropic, Gemini, or OpenAI API key to enable agent reasoning.'),
      actionLabel: t('onboarding.step1Action', 'Configure Keys'),
      action: () => onNavigateTab('settings'),
      completed: false,
    },
    {
      id: 2,
      icon: Bot,
      title: t('onboarding.step2Title', 'Step 2: Meet Nova'),
      desc: t('onboarding.step2Desc', 'The built-in system operator is ready to assist with tool calling and workspace files.'),
      actionLabel: t('onboarding.step2Action', 'Start Instant Chat'),
      action: () => onOpenChat('agent_system_core'),
      completed: true,
    },
    {
      id: 3,
      icon: Wrench,
      title: t('onboarding.step3Title', 'Step 3: Connect Tools & MCP'),
      desc: t('onboarding.step3Desc', 'Discover native tools, connect MCP servers over stdio, and load WASM plugins.'),
      actionLabel: t('onboarding.step3Action', 'Explore Tools'),
      action: () => onNavigateTab('tools'),
      completed: false,
    },
    {
      id: 4,
      icon: ShieldCheck,
      title: t('onboarding.step4Title', 'Step 4: Enable Remote Access'),
      desc: t('onboarding.step4Desc', 'Connect embedded Tailscale (tsnet) to control ActonOS securely from anywhere.'),
      actionLabel: t('onboarding.step4Action', 'Set Tailscale'),
      action: () => onNavigateTab('settings'),
      completed: false,
    },
  ];

  const completedCount = steps.filter((s) => s.completed).length;
  const progressPercent = Math.round((completedCount / steps.length) * 100);

  return (
    <Card className="mb-8 p-5 sm:p-6 border border-onyx/15 shadow-sm bg-gradient-to-br from-soft-meadow via-soft-meadow to-canvas relative overflow-hidden">
      {/* Background Subtle Accent Blob */}
      <div className="absolute -top-16 -right-16 w-48 h-48 rounded-full bg-hi-yellow/20 blur-2xl pointer-events-none" />

      {/* Header Banner */}
      <div className="flex items-start justify-between gap-4 mb-4">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center shadow-xs shrink-0">
            <Sparkles className="w-5 h-5" />
          </div>
          <div>
            <h2 className="font-serif text-heading-sm text-deep-ink tracking-tight flex items-center gap-2">
              <span>{t('onboarding.title', 'Welcome to ActonOS — Quickstart Guide')}</span>
            </h2>
            <p className="font-sans text-body-sm text-slate mt-0.5">
              {t('onboarding.subtitle', 'Follow these 4 simple steps to configure your appliance and unleash autonomous AI agents.')}
            </p>
          </div>
        </div>

        {/* Controls */}
        <div className="flex items-center gap-1 shrink-0">
          <button
            onClick={() => setCollapsed(!collapsed)}
            className="p-1.5 rounded-full hover:bg-black/5 text-slate hover:text-deep-ink transition-colors"
            title={collapsed ? 'Expand' : 'Collapse'}
          >
            {collapsed ? <ChevronDown className="w-4 h-4" /> : <ChevronUp className="w-4 h-4" />}
          </button>
          <button
            onClick={handleDismiss}
            className="p-1.5 rounded-full hover:bg-black/5 text-slate hover:text-deep-ink transition-colors"
            title="Dismiss onboarding"
          >
            <X className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* Progress Bar */}
      <div className="mb-4">
        <div className="flex items-center justify-between text-caption font-medium text-slate mb-1.5">
          <span>{t('onboarding.progress', { completed: completedCount, total: steps.length })}</span>
          <span className="font-mono">{progressPercent}%</span>
        </div>
        <div className="w-full h-2 bg-canvas rounded-full overflow-hidden border border-onyx/10">
          <div
            className="h-full bg-deep-ink rounded-full transition-all duration-300"
            style={{ width: `${progressPercent}%` }}
          />
        </div>
      </div>

      {/* Step Cards Grid */}
      {!collapsed && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3 mt-4">
          {steps.map((step) => {
            const Icon = step.icon;
            return (
              <div
                key={step.id}
                className={`p-4 rounded-[18px] flex flex-col justify-between transition-all border ${step.completed
                  ? 'bg-canvas/90 border-emerald-300 shadow-xs'
                  : 'bg-canvas/60 border-onyx/10 hover:border-onyx/20 hover:bg-canvas'
                  }`}
              >
                <div>
                  <div className="flex items-center justify-between mb-2.5">
                    <div
                      className={`w-8 h-8 rounded-full flex items-center justify-center ${step.completed
                        ? 'bg-emerald-100 text-emerald-700'
                        : 'bg-soft-meadow text-deep-ink border border-onyx/10'
                        }`}
                    >
                      <Icon className="w-4 h-4" />
                    </div>
                    {step.completed ? (
                      <CheckCircle2 className="w-4 h-4 text-emerald-600" />
                    ) : (
                      <span className="text-caption font-mono text-slate font-semibold">#{step.id}</span>
                    )}
                  </div>

                  <h3 className="font-sans text-body-sm font-semibold text-deep-ink mb-1">
                    {step.title}
                  </h3>
                  <p className="font-sans text-caption text-slate line-clamp-2 mb-3">
                    {step.desc}
                  </p>
                </div>

                <Button
                  variant={step.completed ? 'ghost' : 'secondary'}
                  size="sm"
                  icon={<ArrowRight className="w-3.5 h-3.5" />}
                  onClick={step.action}
                  className="w-full justify-center text-caption py-1.5"
                >
                  {step.actionLabel}
                </Button>
              </div>
            );
          })}
        </div>
      )}
    </Card>
  );
}
