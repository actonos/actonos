import { Shield } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { ApprovalLevel } from '@/lib/types';
import { Card } from '@/components/ui/Card';
import { Input } from '@/components/ui/Input';

export function AgentGovernanceSection({
  budget,
  approvalLevel,
  allowedPaths,
  onBudgetChange,
  onApprovalLevelChange,
  onAllowedPathsChange,
}: {
  budget: number;
  approvalLevel: ApprovalLevel;
  allowedPaths: string;
  onBudgetChange: (value: number) => void;
  onApprovalLevelChange: (value: ApprovalLevel) => void;
  onAllowedPathsChange: (value: string) => void;
}) {
  const { t } = useTranslation('agents');
  return (
    <Card className="space-y-6 border border-onyx/15 bg-soft-meadow p-6 shadow-xs">
      <h3 className="flex items-center gap-2 font-serif text-heading-sm text-deep-ink">
        <Shield className="h-5 w-5" aria-hidden="true" />
        {t('studio.governance.title')}
      </h3>
      <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
        <label className="text-caption font-semibold text-deep-ink">
          <span className="mb-1 block">{t('studio.governance.budget')}</span>
          <Input type="number" min={0} value={budget} onChange={(event) => onBudgetChange(Number(event.target.value) || 0)} />
          <span className="mt-1 block text-[11px] font-normal text-slate">{t('studio.governance.budgetHelp')}</span>
        </label>
        <label className="text-caption font-semibold text-deep-ink">
          <span className="mb-1 block">{t('studio.governance.approval')}</span>
          <select
            value={approvalLevel}
            onChange={(event) => onApprovalLevelChange(event.target.value as ApprovalLevel)}
            className="density-control w-full rounded-full border border-onyx/20 bg-canvas px-4 text-body-sm text-deep-ink focus:border-deep-ink focus:outline-none focus:ring-2 focus:ring-deep-ink/10 transition-all"
          >
            <option value="Low">{t('studio.governance.low')}</option>
            <option value="Medium">{t('studio.governance.medium')}</option>
            <option value="High">{t('studio.governance.high')}</option>
          </select>
        </label>
      </div>
      <label className="text-caption font-semibold text-deep-ink">
        <span className="mb-1 block">{t('studio.governance.paths')}</span>
        <Input value={allowedPaths} onChange={(event) => onAllowedPathsChange(event.target.value)} placeholder={t('studio.governance.pathsPlaceholder')} />
        <span className="mt-1 block text-[11px] font-normal text-slate">{t('studio.governance.pathsHelp')}</span>
      </label>
    </Card>
  );
}
