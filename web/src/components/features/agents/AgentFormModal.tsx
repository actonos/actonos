import { useState, useEffect, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Input } from '@/components/ui/Input';
import { Button } from '@/components/ui/Button';
import { LATEST_MODEL_CATALOG } from '@/lib/models';
import type { AgentManifest, ApprovalLevel, ToolInfo } from '@/lib/types';

export interface AgentFormModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSubmit: (data: Partial<AgentManifest>) => Promise<void>;
  initialAgent?: AgentManifest | null;
  availableTools: ToolInfo[];
}

export function AgentFormModal({
  isOpen,
  onClose,
  onSubmit,
  initialAgent,
  availableTools,
}: AgentFormModalProps) {
  const { t } = useTranslation('agents');
  const { t: tCommon } = useTranslation('common');

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [systemInstructions, setSystemInstructions] = useState('');
  const [primaryModel, setPrimaryModel] = useState('openai/gpt-5.4-mini');
  const [fallbackModel, setFallbackModel] = useState('openai/gpt-5.4-mini');
  const [authorizedTools, setAuthorizedTools] = useState<string[]>(['native_file_read', 'native_sysinfo']);
  const [monthlyBudget, setMonthlyBudget] = useState(50);
  const [approvalLevel, setApprovalLevel] = useState<ApprovalLevel>('Medium');
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (initialAgent) {
      setName(initialAgent.name || '');
      setDescription(initialAgent.description || '');
      setSystemInstructions(initialAgent.system_instructions || '');
      setPrimaryModel(initialAgent.model_config?.primary_model || 'openai/gpt-5.4-mini');
      setFallbackModel(initialAgent.model_config?.fallback_model || 'openai/gpt-5.4-mini');
      setAuthorizedTools(initialAgent.authorized_tools || ['native_file_read', 'native_sysinfo']);
      setMonthlyBudget(initialAgent.delegation_scope?.max_monthly_budget_usd ?? 50);
      setApprovalLevel(initialAgent.delegation_scope?.require_human_approval_level || 'Medium');
    } else {
      setName('');
      setDescription('');
      setSystemInstructions('');
      setPrimaryModel('openai/gpt-5.4-mini');
      setFallbackModel('openai/gpt-5.4-mini');
      setAuthorizedTools(['native_file_read', 'native_sysinfo']);
      setMonthlyBudget(50);
      setApprovalLevel('Medium');
    }
  }, [initialAgent, isOpen]);

  const toggleTool = (toolName: string) => {
    setAuthorizedTools((prev) =>
      prev.includes(toolName) ? prev.filter((t) => t !== toolName) : [...prev, toolName]
    );
  };

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;

    setLoading(true);
    try {
      await onSubmit({
        name: name.trim(),
        description: description.trim(),
        system_instructions: systemInstructions.trim(),
        model_config: {
          primary_model: primaryModel,
          fallback_model: fallbackModel,
          reasoning_effort: 'medium',
        },
        authorized_tools: authorizedTools,
        delegation_scope: {
          max_monthly_budget_usd: monthlyBudget,
          allowed_workspace_paths: ['*'],
          require_human_approval_level: approvalLevel,
        },
        listen_channels: initialAgent?.listen_channels || ['*'],
      });
      onClose();
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={initialAgent ? t('modal.editTitle') : t('modal.createTitle')}
    >
      <form onSubmit={handleSubmit} className="space-y-5">
        {/* Name & Description */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <Input
            label={t('modal.nameLabel')}
            placeholder={t('modal.namePlaceholder')}
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
          />
          <Input
            label={t('modal.descriptionLabel')}
            placeholder={t('modal.descriptionPlaceholder')}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </div>

        {/* System Instructions */}
        <div className="flex flex-col gap-1.5">
          <label className="text-caption uppercase tracking-wider text-slate font-medium">
            {t('modal.systemInstructionsLabel')}
          </label>
          <textarea
            rows={4}
            value={systemInstructions}
            onChange={(e) => setSystemInstructions(e.target.value)}
            placeholder={t('modal.systemInstructionsPlaceholder')}
            className="w-full bg-white text-deep-ink font-mono text-body-sm p-3.5 rounded-2xl border border-onyx focus:outline-none focus:ring-2 focus:ring-deep-ink"
          />
        </div>

        {/* Models Row */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div className="flex flex-col gap-1.5">
            <label className="text-caption uppercase tracking-wider text-slate font-medium">
              {t('modal.primaryModelLabel')}
            </label>
            <select
              value={primaryModel}
              onChange={(e) => setPrimaryModel(e.target.value)}
              className="w-full bg-white text-deep-ink font-sans text-body px-4 py-2.5 rounded-full border border-onyx focus:outline-none focus:ring-2 focus:ring-deep-ink"
            >
              {LATEST_MODEL_CATALOG.map((m) => (
                <option key={m.id} value={m.id}>
                  {m.providerName} — {m.name} {m.badge ? `(${m.badge})` : ''}
                </option>
              ))}
            </select>
          </div>

          <div className="flex flex-col gap-1.5">
            <label className="text-caption uppercase tracking-wider text-slate font-medium">
              {t('modal.fallbackModelLabel')}
            </label>
            <select
              value={fallbackModel}
              onChange={(e) => setFallbackModel(e.target.value)}
              className="w-full bg-white text-deep-ink font-sans text-body px-4 py-2.5 rounded-full border border-onyx focus:outline-none focus:ring-2 focus:ring-deep-ink"
            >
              {LATEST_MODEL_CATALOG.map((m) => (
                <option key={m.id} value={m.id}>
                  {m.providerName} — {m.name}
                </option>
              ))}
            </select>
          </div>
        </div>

        {/* Authorized Tools Selector */}
        <div className="flex flex-col gap-2">
          <label className="text-caption uppercase tracking-wider text-slate font-medium">
            {t('modal.toolsLabel')}
          </label>
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-2 max-h-40 overflow-y-auto p-3 bg-soft-meadow rounded-[16px]">
            {availableTools.map((tool) => (
              <label
                key={tool.name}
                className="flex items-center gap-2 text-body-sm font-sans text-deep-ink cursor-pointer select-none"
              >
                <input
                  type="checkbox"
                  checked={authorizedTools.includes(tool.name)}
                  onChange={() => toggleTool(tool.name)}
                  className="rounded accent-deep-ink w-4 h-4 cursor-pointer"
                />
                <span className="truncate text-body-sm" title={tool.name}>
                  {tool.name}
                </span>
              </label>
            ))}
          </div>
        </div>

        {/* Delegation & Budget */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div className="flex flex-col gap-1.5">
            <label className="text-caption uppercase tracking-wider text-slate font-medium">
              {t('modal.budgetLabel')}
            </label>
            <input
              type="number"
              min="0"
              value={monthlyBudget}
              onChange={(e) => setMonthlyBudget(parseFloat(e.target.value) || 0)}
              className="w-full bg-white text-deep-ink font-sans text-body px-5 py-2.5 rounded-full border border-onyx focus:outline-none focus:ring-2 focus:ring-deep-ink"
            />
          </div>

          <div className="flex flex-col gap-1.5">
            <label className="text-caption uppercase tracking-wider text-slate font-medium">
              {t('modal.approvalLabel')}
            </label>
            <select
              value={approvalLevel}
              onChange={(e) => setApprovalLevel(e.target.value as ApprovalLevel)}
              className="w-full bg-white text-deep-ink font-sans text-body px-4 py-2.5 rounded-full border border-onyx focus:outline-none focus:ring-2 focus:ring-deep-ink"
            >
              <option value="Low">{t('modal.approvalLow')}</option>
              <option value="Medium">{t('modal.approvalMed')}</option>
              <option value="High">{t('modal.approvalHigh')}</option>
            </select>
          </div>
        </div>

        {/* Submit Pair */}
        <div className="flex items-center justify-end gap-3 pt-4 border-t border-soft-meadow">
          <Button type="button" variant="ghost" onClick={onClose}>
            {tCommon('buttons.cancel')}
          </Button>
          <Button type="submit" variant="primary" disabled={loading}>
            {loading ? '...' : tCommon('buttons.save')}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
