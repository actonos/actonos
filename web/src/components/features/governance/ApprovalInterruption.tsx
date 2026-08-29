import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '@/lib/api';
import { isModalEligibleApproval, type ApprovalRequest, type DontAskAgain } from '@/lib/types';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { useToast } from '@/components/ui/Toast';
import { useRealtime } from '@/components/providers/RealtimeProvider';
import { ApprovalDecisionBar } from './ApprovalDecisionBar';

export function ApprovalInterruption() {
  const { t } = useTranslation('operations');
  const { error, info, success } = useToast();
  const [approval, setApproval] = useState<ApprovalRequest | null>(null);
  const [feedback, setFeedback] = useState('');
  const [deciding, setDeciding] = useState(false);
  const dialogRef = useRef<HTMLDivElement>(null);
  const feedbackRef = useRef<HTMLTextAreaElement>(null);
  const { snapshot } = useRealtime();

  useEffect(() => {
    setApproval((current) => {
      if (current && isModalEligibleApproval(current)) return current;
      return snapshot?.approvals?.find(isModalEligibleApproval) || null;
    });
  }, [snapshot?.approvals]);

  useEffect(() => {
    const handleApprovalRequired = (event: Event) => {
      const next = (event as CustomEvent<ApprovalRequest>).detail;
      if (!next || !isModalEligibleApproval(next)) return;
      setApproval(next);
    };
    window.addEventListener('actonos:approval-required', handleApprovalRequired);
    return () => {
      window.removeEventListener('actonos:approval-required', handleApprovalRequired);
    };
  }, []);

  useEffect(() => {
    if (!approval) return;
    feedbackRef.current?.focus();
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Tab' || !dialogRef.current) return;
      const focusable = dialogRef.current.querySelectorAll<HTMLElement>(
        'button:not([disabled]), textarea:not([disabled])',
      );
      if (!focusable.length) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [approval]);

  if (!approval) return null;

  const decide = async (approved: boolean, dontAskAgain?: DontAskAgain) => {
    setDeciding(true);
    try {
      if (approved) {
        await api.approveAction(approval.id, feedback.trim() || t('approval.approvedFeedback'), dontAskAgain);
        success(
          t('approval.approved'),
          dontAskAgain === 'today'
            ? t('approval.grantedToday', { tool: approval.tool_name })
            : dontAskAgain === 'task'
              ? t('approval.grantedTask', { tool: approval.tool_name })
              : approval.tool_name
        );
        window.dispatchEvent(
          new CustomEvent('actonos:approval-decided', {
            detail: { id: approval.id, approved: true, tool: approval.tool_name },
          })
        );
        window.dispatchEvent(new CustomEvent('actonos:tools-updated'));
      } else {
        await api.rejectAction(approval.id, feedback.trim() || t('approval.defaultFeedback'));
        info(t('approval.rejected'), approval.tool_name);
        window.dispatchEvent(
          new CustomEvent('actonos:approval-decided', {
            detail: { id: approval.id, approved: false, tool: approval.tool_name },
          })
        );
      }
      setApproval(null);
      setFeedback('');
    } catch (cause) {
      error(t('approval.failed'), cause instanceof Error ? cause.message : String(cause));
      // A failed decision may have left this record decided, expired, or
      // reopened server-side. Re-read the queue so the dialog reflects reality
      // instead of pinning a stale approval the operator cannot resolve.
      const refreshed = await api
        .listApprovals('pending')
        .then((result) => result.approvals?.[0] || null)
        .catch(() => null);
      setApproval(refreshed);
      setFeedback('');
    } finally {
      setDeciding(false);
    }
  };

  const riskTier = approval.risk_tier || (approval.risk_level?.toLowerCase() === 'high' ? 'high' : approval.risk_level?.toLowerCase() === 'medium' ? 'medium' : 'low');
  const riskTone = riskTier === 'high' ? 'stopped' : riskTier === 'medium' ? 'warning' : 'success';
  const autoApproveSec = typeof approval.auto_approve_after === 'number' ? approval.auto_approve_after : parseInt(String(approval.auto_approve_after || 0), 10);
  const autoApproveHours = autoApproveSec > 0 ? (autoApproveSec / 3600).toFixed(1).replace(/\.0$/, '') : null;

  return (
    <div className="fixed inset-0 z-[100] bg-deep-ink/55 backdrop-blur-sm flex items-center justify-center p-4">
      <Card
        ref={dialogRef}
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="approval-interruption-title"
        className="w-full max-w-2xl p-6 bg-canvas border-2 border-deep-ink"
      >
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={riskTone}>
            {riskTier.toUpperCase()} {t('approval.riskTier')}
          </Badge>
          <Badge variant="neutral">{approval.risk_level}</Badge>
          {autoApproveHours ? (
            <span className="text-[11px] font-mono text-slate bg-soft-meadow px-2 py-0.5 rounded-full border border-onyx/10">
              ⏱ {t('approval.autoApproveIn', { time: `${autoApproveHours}h` })}
            </span>
          ) : riskTier === 'high' ? (
            <span className="text-[11px] font-mono text-rose-700 bg-rose-50 px-2 py-0.5 rounded-full border border-rose-200">
              🛡 {t('approval.manualReviewRequired')}
            </span>
          ) : null}
        </div>

        <h2 id="approval-interruption-title" className="font-serif text-heading mt-3 font-bold">{t('approval.title')}</h2>
        <p className="text-body-sm text-slate mt-1">
          {t('approval.description', { tool: approval.tool_name, agent: approval.agent_id })}
        </p>
        <pre className="my-4 max-h-60 overflow-auto rounded-[18px] bg-deep-ink text-white p-4 text-xs">
          {JSON.stringify(approval.input, null, 2)}
        </pre>
        <label className="block text-caption font-semibold text-deep-ink mb-1">{t('approval.feedbackLabel')}</label>
        <textarea
          ref={feedbackRef}
          value={feedback}
          onChange={(event) => setFeedback(event.target.value)}
          placeholder={t('approval.feedbackPlaceholder')}
          rows={3}
          className="w-full mb-4 rounded-[18px] border border-onyx/15 bg-soft-meadow px-4 py-3 text-body-sm"
        />
        <ApprovalDecisionBar
          approval={approval}
          deciding={deciding}
          canReject={Boolean(feedback.trim())}
          labels={{
            reject: t('approval.reject'),
            approve: t('approval.approve'),
            dontAskTask: t('approval.dontAskTask'),
            dontAskToday: t('approval.dontAskToday'),
          }}
          onReject={() => decide(false)}
          onApprove={(dontAskAgain) => decide(true, dontAskAgain)}
        />
      </Card>
    </div>
  );
}
