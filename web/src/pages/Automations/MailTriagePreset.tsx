import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Mail } from 'lucide-react';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { useToast } from '@/components/ui/Toast';
import { api } from '@/lib/api';
import { buildTriageAgent, buildTriageJob, TRIAGE_AGENT_ID, type TriageInterval } from '@/lib/mailTriage';
import { getErrorMessage } from '@/lib/errors';
import type { ModelConfig } from '@/lib/types';

const INTERVALS: TriageInterval[] = [5, 15, 30, 60];

/**
 * Starts a recurring look at the inbox. Mail triggers nothing by itself in this version; this is
 * the one place where the owner turns polling on, and the agent it creates cannot send unless
 * the owner says so (and even then the approval gate and rules still decide each send).
 */
export function MailTriagePreset({ hasAccount, onCreated }: { hasAccount: boolean; onCreated?: () => void }) {
  const { t } = useTranslation('automations');
  const { success, error } = useToast();
  const [minutes, setMinutes] = useState<TriageInterval>(15);
  const [autoSend, setAutoSend] = useState(false);
  const [busy, setBusy] = useState(false);

  const start = async () => {
    setBusy(true);
    try {
      let modelConfig: ModelConfig = { primary_model: 'openai/gpt-5.4-mini' };
      try {
        const core = await api.getAgent('agent_system_core');
        if (core.model_config?.primary_model) modelConfig = core.model_config;
      } catch {
        // Fall back to the default model when the core agent cannot be read.
      }
      const opts = {
        everyMinutes: minutes,
        allowAutoSend: autoSend,
        modelConfig,
        texts: { instructions: t('mailTriage.instructions'), prompt: t('mailTriage.prompt'), autoSendClause: t('mailTriage.autoSendClause') },
      };
      try {
        await api.createAgent(buildTriageAgent(opts));
      } catch {
        // The agent may already exist from an earlier run; update it so the tool list matches the choice.
        await api.updateAgent(TRIAGE_AGENT_ID, buildTriageAgent(opts));
      }
      await api.saveCronJob({ ...buildTriageJob(opts), target_channel: 'none' });
      success(t('mailTriage.started'));
      onCreated?.();
    } catch (cause) {
      error(t('mailTriage.failed'), getErrorMessage(cause));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card className="mb-6 flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <Mail className="h-4 w-4 text-slate" aria-hidden="true" />
        <h2 className="font-serif text-heading-sm font-semibold text-deep-ink">{t('mailTriage.title')}</h2>
      </div>
      <p className="text-body-sm text-slate">{hasAccount ? t('mailTriage.description') : t('mailTriage.noAccount')}</p>
      <div className="flex flex-wrap items-center gap-4">
        <label className="flex items-center gap-2 text-body-sm text-deep-ink">
          {t('mailTriage.every')}
          <select
            aria-label={t('mailTriage.every')}
            value={minutes}
            onChange={(e) => setMinutes(Number(e.target.value) as TriageInterval)}
            className="rounded-full border border-onyx/15 bg-canvas px-3 py-1.5 text-body-sm"
          >
            {INTERVALS.map((m) => (
              <option key={m} value={m}>{t('mailTriage.minutes', { count: m })}</option>
            ))}
          </select>
        </label>
        <label className="flex items-center gap-2 text-body-sm text-deep-ink">
          <input type="checkbox" checked={autoSend} onChange={(e) => setAutoSend(e.target.checked)} />
          {t('mailTriage.allowAutoSend')}
        </label>
        <Button size="sm" disabled={!hasAccount || busy} onClick={() => void start()}>{t('mailTriage.start')}</Button>
      </div>
      {autoSend && <p className="text-caption text-status-warning">{t('mailTriage.autoSendNote')}</p>}
    </Card>
  );
}
