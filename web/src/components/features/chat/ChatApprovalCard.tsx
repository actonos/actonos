import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '@/lib/api';
import type { ApprovalRequest, DontAskAgain } from '@/lib/types';
import { Badge } from '@/components/ui/Badge';
import { useToast } from '@/components/ui/Toast';
import { ApprovalDecisionBar } from '@/components/features/governance/ApprovalDecisionBar';
import { getErrorMessage } from '@/lib/errors';

export function ChatApprovalCard({
  approval,
  onDecided,
}: {
  approval: ApprovalRequest;
  onDecided?: (approved: boolean) => void;
}) {
  const { t } = useTranslation('chat');
  const { t: tOps } = useTranslation('operations');
  const { error, info, success } = useToast();
  const [feedback, setFeedback] = useState('');
  const [deciding, setDeciding] = useState(false);
  const feedbackRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    feedbackRef.current?.focus();
  }, [approval.id]);

  const decide = async (approved: boolean, dontAskAgain?: DontAskAgain) => {
    if (!approved && !feedback.trim()) return;
    setDeciding(true);
    try {
      if (approved) {
        await api.approveAction(
          approval.id,
          feedback.trim() || tOps('approval.approvedFeedback'),
          dontAskAgain
        );
        success(
          tOps('approval.approved'),
          dontAskAgain === 'today'
            ? tOps('approval.grantedToday', { tool: approval.tool_name })
            : dontAskAgain === 'task'
              ? tOps('approval.grantedTask', { tool: approval.tool_name })
              : approval.tool_name
        );
        window.dispatchEvent(
          new CustomEvent('actonos:approval-decided', {
            detail: { id: approval.id, approved: true, tool: approval.tool_name },
          })
        );
        window.dispatchEvent(new CustomEvent('actonos:tools-updated'));
      } else {
        await api.rejectAction(approval.id, feedback.trim());
        info(tOps('approval.rejected'), approval.tool_name);
        window.dispatchEvent(
          new CustomEvent('actonos:approval-decided', {
            detail: { id: approval.id, approved: false, tool: approval.tool_name },
          })
        );
      }
      onDecided?.(approved);
    } catch (cause) {
      error(tOps('approval.failed'), getErrorMessage(cause));
    } finally {
      setDeciding(false);
    }
  };

  const riskVariant = approval.risk_level === 'High' ? 'danger' : approval.risk_level === 'Medium' ? 'warning' : 'neutral';

  return (
    <section
      className="mt-3 rounded-[18px] border border-onyx/15 bg-canvas p-4"
      aria-labelledby={`chat-approval-title-${approval.id}`}
    >
      <Badge variant={riskVariant}>{approval.risk_level}</Badge>
      <h3 id={`chat-approval-title-${approval.id}`} className="mt-2 font-serif text-body font-bold text-deep-ink">
        {t('approvalTitle')}
      </h3>
      <p className="mt-1 text-caption text-slate">
        {t('approvalDescription', { tool: approval.tool_name, agent: approval.agent_id })}
      </p>
      <pre className="my-3 max-h-40 overflow-auto rounded-[18px] bg-deep-ink p-3 text-[11px] text-white">
        {JSON.stringify(approval.input ?? {}, null, 2)}
      </pre>
      <label className="mb-1 block text-caption font-semibold text-deep-ink" htmlFor={`chat-approval-feedback-${approval.id}`}>
        {tOps('approval.feedbackLabel')}
      </label>
      <textarea
        id={`chat-approval-feedback-${approval.id}`}
        ref={feedbackRef}
        value={feedback}
        onChange={(event) => setFeedback(event.target.value)}
        placeholder={tOps('approval.feedbackPlaceholder')}
        rows={3}
        className="mb-3 w-full rounded-[18px] border border-onyx/15 bg-soft-meadow px-4 py-3 text-body-sm"
      />
      <ApprovalDecisionBar
        approval={approval}
        deciding={deciding}
        canReject={Boolean(feedback.trim())}
        labels={{
          reject: tOps('approval.reject'),
          approve: tOps('approval.approve'),
          dontAskTask: tOps('approval.dontAskTask'),
          dontAskToday: tOps('approval.dontAskToday'),
        }}
        onReject={() => {
          void decide(false);
        }}
        onApprove={(dontAskAgain) => {
          void decide(true, dontAskAgain);
        }}
      />
    </section>
  );
}
