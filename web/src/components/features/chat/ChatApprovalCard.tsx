import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '@/lib/api';
import type { ApprovalRequest } from '@/lib/types';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { useToast } from '@/components/ui/Toast';
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
  const [deciding, setDeciding] = useState(false);
  const rootRef = useRef<HTMLElement>(null);

  useEffect(() => {
    rootRef.current?.scrollIntoView({ behavior: 'smooth', block: 'end', inline: 'nearest' });
  }, [approval.id]);

  const decide = async (approved: boolean) => {
    setDeciding(true);
    try {
      if (approved) {
        await api.approveAction(approval.id, t('approvalApprovedFeedback'));
        success(tOps('approval.approved'), approval.tool_name);
        window.dispatchEvent(
          new CustomEvent('actonos:approval-decided', {
            detail: { id: approval.id, approved: true, tool: approval.tool_name },
          })
        );
        window.dispatchEvent(new CustomEvent('actonos:tools-updated'));
      } else {
        await api.rejectAction(approval.id, t('approvalRejectedFeedback'));
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
      ref={rootRef}
      className="sticky bottom-0 z-20 mt-3 scroll-mt-6 rounded-full border border-onyx/15 bg-canvas px-3 py-2"
      aria-label={t('approvalTitle')}
      aria-live="polite"
    >
      <div className="flex items-center gap-2">
        <Badge variant={riskVariant} className="px-2 py-0 text-[10px] shrink-0">
          {approval.risk_level}
        </Badge>
        <p className="min-w-0 flex-1 truncate font-mono text-[11px] font-semibold text-deep-ink" title={approval.tool_name}>
          {approval.tool_name}
        </p>
        <div className="flex shrink-0 items-center gap-1.5">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={deciding}
            onClick={() => {
              void decide(false);
            }}
          >
            {t('approvalReject')}
          </Button>
          <Button
            type="button"
            variant="primary"
            size="sm"
            disabled={deciding}
            onClick={() => {
              void decide(true);
            }}
          >
            {t('approvalApprove')}
          </Button>
        </div>
      </div>
    </section>
  );
}
