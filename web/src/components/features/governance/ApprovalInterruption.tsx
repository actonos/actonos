import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '@/lib/api';
import type { ApprovalRequest } from '@/lib/types';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { useToast } from '@/components/ui/Toast';
import { useRealtime } from '@/components/providers/RealtimeProvider';

export function ApprovalInterruption() {
  const { t } = useTranslation('operations');
  const { error, info, success } = useToast();
  const [approval, setApproval] = useState<ApprovalRequest | null>(null);
  const [feedback, setFeedback] = useState('');
  const [deciding, setDeciding] = useState(false);
  const { snapshot } = useRealtime();

  useEffect(() => {
    setApproval((current) => current || snapshot?.approvals?.[0] || null);
  }, [snapshot?.approvals]);

  useEffect(() => {
    const handleApprovalRequired = (event: Event) => {
      setApproval((event as CustomEvent<ApprovalRequest>).detail);
    };
    window.addEventListener('actonos:approval-required', handleApprovalRequired);
    return () => {
      window.removeEventListener('actonos:approval-required', handleApprovalRequired);
    };
  }, []);

  if (!approval) return null;

  const decide = async (approved: boolean) => {
    setDeciding(true);
    try {
      if (approved) {
        await api.approveAction(approval.id, t('approval.approvedFeedback'));
        success(t('approval.approved'), approval.tool_name);
      } else {
        await api.rejectAction(approval.id, feedback.trim() || t('approval.defaultFeedback'));
        info(t('approval.rejected'), approval.tool_name);
      }
      setApproval(null);
      setFeedback('');
    } catch (cause) {
      error(t('approval.failed'), cause instanceof Error ? cause.message : String(cause));
    } finally {
      setDeciding(false);
    }
  };

  return (
    <div className="fixed inset-0 z-[100] bg-deep-ink/55 backdrop-blur-sm flex items-center justify-center p-4">
      <Card
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="approval-interruption-title"
        className="w-full max-w-2xl p-6 bg-canvas border-2 border-deep-ink"
      >
        <Badge variant="stopped">{approval.risk_level}</Badge>
        <h2 id="approval-interruption-title" className="font-serif text-heading mt-3 font-bold">{t('approval.title')}</h2>
        <p className="text-body-sm text-slate mt-1">
          {t('approval.description', { tool: approval.tool_name, agent: approval.agent_id })}
        </p>
        <pre className="my-4 max-h-60 overflow-auto rounded-[18px] bg-deep-ink text-canvas p-4 text-xs">
          {JSON.stringify(approval.input, null, 2)}
        </pre>
        <label className="block text-caption font-semibold text-deep-ink mb-1">{t('approval.feedbackLabel')}</label>
        <textarea
          value={feedback}
          onChange={(event) => setFeedback(event.target.value)}
          placeholder={t('approval.feedbackPlaceholder')}
          rows={3}
          className="w-full mb-4 rounded-[18px] border border-onyx/15 bg-soft-meadow px-4 py-3 text-body-sm"
        />
        <div className="flex flex-col-reverse sm:flex-row justify-end gap-2">
          <Button variant="ghost" disabled={deciding} onClick={() => decide(false)}>{t('approval.reject')}</Button>
          <Button variant="primary" disabled={deciding} onClick={() => decide(true)}>{t('approval.approve')}</Button>
        </div>
      </Card>
    </div>
  );
}
