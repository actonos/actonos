import { Button } from '@/components/ui/Button';
import type { ApprovalRequest, DontAskAgain } from '@/lib/types';

export function ApprovalDecisionBar({
  approval,
  deciding,
  canReject = true,
  labels,
  onReject,
  onApprove,
}: {
  approval: ApprovalRequest;
  deciding: boolean;
  canReject?: boolean;
  labels: {
    reject: string;
    approve: string;
    dontAskTask: string;
    dontAskToday: string;
  };
  onReject: () => void;
  onApprove: (dontAskAgain?: DontAskAgain) => void;
}) {
  const canGrant =
    Boolean(approval.tool_name) &&
    !approval.tool_name.startsWith('admin_') &&
    approval.tool_name !== 'system_mcp_connect';
  const hasTask = Boolean(approval.task_id);

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-col-reverse sm:flex-row justify-end gap-2">
        <Button variant="ghost" disabled={deciding || !canReject} onClick={onReject}>
          {labels.reject}
        </Button>
        <Button variant="primary" disabled={deciding} onClick={() => onApprove()}>
          {labels.approve}
        </Button>
      </div>
      {canGrant && (
        <div className="flex flex-col sm:flex-row sm:justify-end gap-2">
          {hasTask && (
            <Button variant="secondary" size="sm" disabled={deciding} onClick={() => onApprove('task')}>
              {labels.dontAskTask}
            </Button>
          )}
          <Button variant="secondary" size="sm" disabled={deciding} onClick={() => onApprove('today')}>
            {labels.dontAskToday}
          </Button>
        </div>
      )}
    </div>
  );
}
