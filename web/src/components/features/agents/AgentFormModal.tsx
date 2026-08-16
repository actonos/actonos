import { useState, useEffect, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Input } from '@/components/ui/Input';
import { Button } from '@/components/ui/Button';
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
  const [primaryModel, setPrimaryModel] = useState('anthropic/claude-3-7-sonnet');
  const [fallbackModel, setFallbackModel] = useState('google/gemini-2.5-flash');
  const [temperature, setTemperature] = useState(0.2);
  const [authorizedTools, setAuthorizedTools] = useState<string[]>(['native_file_read', 'native_sysinfo']);
  const [monthlyBudget, setMonthlyBudget] = useState(50);
  const [approvalLevel, setApprovalLevel] = useState<ApprovalLevel>('Medium');
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (initialAgent) {
      setName(initialAgent.name || '');
      setDescription(initialAgent.description || '');
      setSystemInstructions(initialAgent.system_instructions || '');
      setPrimaryModel(initialAgent.model_config?.primary_model || 'anthropic/claude-3-7-sonnet');
      setFallbackModel(initialAgent.model_config?.fallback_model || 'google/gemini-2.5-flash');
      setTemperature(initialAgent.model_config?.temperature ?? 0.2);
      setAuthorizedTools(initialAgent.authorized_tools || []);
      setMonthlyBudget(initialAgent.delegation_scope?.max_monthly_budget_usd || 50);
      setApprovalLevel(initialAgent.delegation_scope?.require_human_approval_level || 'Medium');
    } else {
      setName('');
      setDescription('');
      setSystemInstructions('You are an autonomous AI Agent running on ActonOS. Always verify actions before execution.');
      setPrimaryModel('anthropic/claude-3-7-sonnet');
      setFallbackModel('google/gemini-2.5-flash');
      setTemperature(0.2);
      setAuthorizedTools(['native_file_read', 'native_sysinfo']);
      setMonthlyBudget(50);
      setApprovalLevel('Medium');
    }
  }, [initialAgent, isOpen]);

  const toggleTool = (toolName: string) => {
    if (authorizedTools.includes(toolName)) {
      setAuthorizedTools(authorizedTools.filter((t) => t !== toolName));
    } else {
      setAuthorizedTools([...authorizedTools, toolName]);
    }
  };

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setLoading(true);
    try {
      await onSubmit({
        name,
        description,
        system_instructions: systemInstructions,
        model_config: {
          primary_model: primaryModel,
          fallback_model: fallbackModel,
          temperature,
        },
        authorized_tools: authorizedTools,
        delegation_scope: {
          max_monthly_budget_usd: monthlyBudget,
          allowed_workspace_paths: ['/data/workspace'],
          require_human_approval_level: approvalLevel,
        },
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
      <form onSubmit={handleSubmit} className="flex flex-col gap-6">
        {/* Basic Info */}
        <Input
          label={t('modal.nameLabel')}
          placeholder={t('modal.namePlaceholder')}
          value={name}
          onChange={(e) => setName(e.target.value)}
          required
        />

        <Input
          label={t('modal.descLabel')}
          placeholder={t('modal.descPlaceholder')}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />

        {/* System Prompt */}
        <div className="flex flex-col gap-1.5">
          <label className="text-caption uppercase tracking-wider text-slate font-medium">
            {t('modal.instructionsLabel')}
          </label>
          <textarea
            rows={4}
            className="w-full bg-white text-deep-ink font-sans text-body p-4 rounded-[20px] border border-onyx focus:outline-none focus:ring-2 focus:ring-deep-ink"
            placeholder={t('modal.instructionsPlaceholder')}
            value={systemInstructions}
            onChange={(e) => setSystemInstructions(e.target.value)}
            required
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
              <option value="anthropic/claude-3-7-sonnet">Claude 3.7 Sonnet (Anthropic)</option>
              <option value="google/gemini-2.5-flash">Gemini 2.5 Flash (Google)</option>
              <option value="openai/gpt-4o">GPT-4o (OpenAI)</option>
              <option value="deepseek/deepseek-r1">DeepSeek R1</option>
              <option value="ollama/llama3">Llama 3 (Local Ollama)</option>
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
              <option value="google/gemini-2.5-flash">Gemini 2.5 Flash (Google)</option>
              <option value="openai/gpt-4o-mini">GPT-4o Mini (OpenAI)</option>
              <option value="ollama/llama3">Llama 3 (Local Ollama)</option>
            </select>
          </div>
        </div>

        {/* Temperature */}
        <div className="flex flex-col gap-1.5">
          <div className="flex justify-between items-center">
            <label className="text-caption uppercase tracking-wider text-slate font-medium">
              {t('modal.temperatureLabel')}
            </label>
            <span className="font-semibold text-body-sm">{temperature}</span>
          </div>
          <input
            type="range"
            min="0"
            max="1"
            step="0.05"
            value={temperature}
            onChange={(e) => setTemperature(parseFloat(e.target.value))}
            className="w-full accent-deep-ink cursor-pointer"
          />
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
