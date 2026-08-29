import { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { useToast } from '@/components/ui/Toast';
import { api } from '@/lib/api';
import type { AgentManifest, SelfImprovementProposal } from '@/lib/types';
import {
  Sparkles,
  RotateCw,
  CheckCircle2,
  XCircle,
  Brain,
  Wrench,
  AlertOctagon,
  TrendingUp,
  Clock,
  Check,
} from 'lucide-react';

export interface AgentInsightsModalProps {
  isOpen: boolean;
  onClose: () => void;
  agent: AgentManifest | null;
}

export function AgentInsightsModal({
  isOpen,
  onClose,
  agent,
}: AgentInsightsModalProps) {
  const { t } = useTranslation('agents');
  const { success, error, info } = useToast();

  const [proposals, setProposals] = useState<SelfImprovementProposal[]>([]);
  const [loading, setLoading] = useState(false);
  const [reviewing, setReviewing] = useState(false);
  const [activeFilter, setActiveFilter] = useState<'all' | 'pending' | 'applied' | 'dismissed'>('all');
  const [actingId, setActingId] = useState<string | null>(null);

  const loadInsights = useCallback(async (isBackground = false) => {
    if (!agent) return;
    try {
      if (!isBackground) setLoading(true);
      const res = await api.listAgentInsights(agent.agent_id);
      setProposals(res.proposals || []);
    } catch (err) {
      if (!isBackground) {
        error(t('insights.reviewFailed'), err instanceof Error ? err.message : String(err));
      }
    } finally {
      if (!isBackground) setLoading(false);
    }
  }, [agent, error, t]);

  useEffect(() => {
    if (isOpen && agent) {
      loadInsights(false);
    }
  }, [isOpen, agent, loadInsights]);

  const handleRunSelfReview = async () => {
    if (!agent) return;
    setReviewing(true);
    try {
      const res = await api.triggerSelfReview(agent.agent_id);
      success(t('insights.reviewSuccess', { count: res.count }));
      await loadInsights(false);
    } catch (err) {
      error(t('insights.reviewFailed'), err instanceof Error ? err.message : String(err));
    } finally {
      setReviewing(false);
    }
  };

  const handleApply = async (proposalId: string) => {
    if (!agent) return;
    setActingId(proposalId);
    try {
      await api.applyAgentInsight(agent.agent_id, proposalId);
      success(t('insights.appliedSuccess'));
      await loadInsights(true);
    } catch (err) {
      error('Failed to apply insight', err instanceof Error ? err.message : String(err));
    } finally {
      setActingId(null);
    }
  };

  const handleDismiss = async (proposalId: string) => {
    if (!agent) return;
    setActingId(proposalId);
    try {
      await api.dismissAgentInsight(agent.agent_id, proposalId);
      info(t('insights.dismissedSuccess'));
      await loadInsights(true);
    } catch (err) {
      error('Failed to dismiss insight', err instanceof Error ? err.message : String(err));
    } finally {
      setActingId(null);
    }
  };

  const getCategoryBadge = (category: SelfImprovementProposal['category']) => {
    switch (category) {
      case 'tool_reliability':
        return (
          <Badge variant="stopped" className="flex items-center gap-1">
            <Wrench className="w-3 h-3" />
            {t('insights.categories.tool_reliability')}
          </Badge>
        );
      case 'prompt_clarity':
        return (
          <Badge variant="warning" className="flex items-center gap-1">
            <Brain className="w-3 h-3" />
            {t('insights.categories.prompt_clarity')}
          </Badge>
        );
      case 'task_failure':
        return (
          <Badge variant="stopped" className="flex items-center gap-1">
            <AlertOctagon className="w-3 h-3" />
            {t('insights.categories.task_failure')}
          </Badge>
        );
      case 'performance':
      default:
        return (
          <Badge variant="neutral" className="flex items-center gap-1">
            <TrendingUp className="w-3 h-3" />
            {t('insights.categories.performance')}
          </Badge>
        );
    }
  };

  const filteredProposals = proposals.filter((p) => {
    if (activeFilter === 'all') return true;
    return p.status === activeFilter;
  });

  if (!agent) return null;

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={`${agent.name} — ${t('insights.title')}`}
      maxWidth="max-w-3xl"
    >
      <div className="space-y-5 font-sans">
        {/* Subtitle & Self-Review Trigger */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 p-4 rounded-[20px] bg-soft-meadow border border-onyx/10">
          <div>
            <p className="text-body-sm font-semibold text-deep-ink flex items-center gap-2">
              <Sparkles className="w-4 h-4 text-deep-ink" />
              {t('insights.subtitle')}
            </p>
            <p className="text-[11px] text-slate mt-0.5">
              Target Agent ID: <span className="font-mono font-semibold">{agent.agent_id}</span>
            </p>
          </div>
          <Button
            variant="primary"
            size="sm"
            icon={<RotateCw className={`w-3.5 h-3.5 ${reviewing ? 'animate-spin' : ''}`} />}
            onClick={handleRunSelfReview}
            disabled={reviewing || loading}
          >
            {reviewing ? t('insights.runningReview') : t('insights.runSelfReview')}
          </Button>
        </div>

        {/* Filter Pills */}
        <div className="flex flex-wrap items-center gap-2">
          {(['all', 'pending', 'applied', 'dismissed'] as const).map((filterKey) => {
            const count =
              filterKey === 'all'
                ? proposals.length
                : proposals.filter((p) => p.status === filterKey).length;
            const label =
              filterKey === 'all'
                ? 'All'
                : filterKey === 'pending'
                  ? t('insights.statusPending')
                  : filterKey === 'applied'
                    ? t('insights.statusApplied')
                    : t('insights.statusDismissed');

            return (
              <button
                key={filterKey}
                type="button"
                onClick={() => setActiveFilter(filterKey)}
                className={`px-3 py-1 text-[12px] font-semibold rounded-full border transition-all cursor-pointer ${
                  activeFilter === filterKey
                    ? 'bg-deep-ink text-canvas border-deep-ink shadow-xs'
                    : 'bg-soft-meadow text-slate border-onyx/10 hover:text-deep-ink'
                }`}
              >
                {label} ({count})
              </button>
            );
          })}
        </div>

        {/* Proposals List */}
        {loading && proposals.length === 0 ? (
          <div className="py-12 text-center text-caption text-slate animate-pulse">
            Loading agent insights...
          </div>
        ) : filteredProposals.length === 0 ? (
          <div className="py-12 text-center rounded-[20px] bg-soft-meadow border border-onyx/5 space-y-2">
            <div className="w-10 h-10 rounded-full bg-deep-ink/5 mx-auto flex items-center justify-center">
              <CheckCircle2 className="w-5 h-5 text-emerald-600" />
            </div>
            <p className="text-body-sm font-semibold text-deep-ink">
              {t('insights.noInsights')}
            </p>
            <p className="text-caption text-slate">
              Click &quot;{t('insights.runSelfReview')}&quot; to evaluate recent mission traces for optimization opportunities.
            </p>
          </div>
        ) : (
          <div className="space-y-3.5 max-h-[480px] overflow-y-auto pr-1">
            {filteredProposals.map((item) => {
              const isActing = actingId === item.id;
              const isPending = item.status === 'pending';

              return (
                <div
                  key={item.id}
                  className="p-4.5 rounded-[20px] bg-canvas border border-onyx/10 space-y-3 shadow-2xs hover:border-onyx/20 transition-all"
                >
                  <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
                    <div className="flex flex-wrap items-center gap-2">
                      {getCategoryBadge(item.category)}
                      <h4 className="font-sans font-bold text-body-sm text-deep-ink">
                        {item.title}
                      </h4>
                    </div>

                    <div className="flex items-center gap-2 shrink-0">
                      <span className="text-[10px] font-mono text-slate flex items-center gap-1">
                        <Clock className="w-3 h-3 text-slate" />
                        {new Date(item.created_at).toLocaleDateString()}
                      </span>
                      <Badge
                        variant={
                          item.status === 'applied'
                            ? 'active'
                            : item.status === 'dismissed'
                              ? 'neutral'
                              : 'warning'
                        }
                      >
                        {item.status.toUpperCase()}
                      </Badge>
                    </div>
                  </div>

                  {/* Observation */}
                  <div className="p-3 rounded-xl bg-soft-meadow border border-onyx/5 text-caption text-slate leading-relaxed">
                    <strong className="text-deep-ink block mb-0.5">{t('insights.observation')}:</strong>
                    {item.observation}
                  </div>

                  {/* Recommendation */}
                  <div className="p-3 rounded-xl bg-emerald-50/70 border border-emerald-200/50 text-caption text-emerald-950 leading-relaxed">
                    <strong className="text-emerald-900 block mb-0.5">➔ {t('insights.suggestion')}:</strong>
                    {item.suggestion}
                  </div>

                  {/* Action Buttons for Pending proposals */}
                  {isPending && (
                    <div className="flex justify-end gap-2 pt-1">
                      <Button
                        variant="ghost"
                        size="sm"
                        icon={<XCircle className="w-3.5 h-3.5 text-slate" />}
                        onClick={() => handleDismiss(item.id)}
                        disabled={isActing}
                      >
                        {t('insights.dismiss')}
                      </Button>
                      <Button
                        variant="primary"
                        size="sm"
                        icon={<Check className="w-3.5 h-3.5" />}
                        onClick={() => handleApply(item.id)}
                        disabled={isActing}
                      >
                        {t('insights.apply')}
                      </Button>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}

        {/* Footer */}
        <div className="flex justify-end pt-2 border-t border-onyx/10">
          <Button variant="secondary" onClick={onClose}>
            {t('insights.close')}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
